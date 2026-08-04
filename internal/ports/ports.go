package ports

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"projectdock/internal/model"
	"projectdock/internal/store"
)

type Scanner interface {
	List(context.Context) ([]model.PortListener, error)
}

type LsofScanner struct {
	Path string
}

func NewLsofScanner() *LsofScanner {
	return &LsofScanner{Path: "/usr/sbin/lsof"}
}

func (s *LsofScanner) List(ctx context.Context) ([]model.PortListener, error) {
	command := exec.CommandContext(ctx, s.Path, "-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcn")
	output, err := command.Output()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && len(exitError.Stderr) == 0 && len(output) == 0 {
			return []model.PortListener{}, nil
		}
		return nil, fmt.Errorf("执行 lsof 端口扫描: %w", err)
	}
	return ParseLsof(output)
}

func ParseLsof(output []byte) ([]model.PortListener, error) {
	var listeners []model.PortListener
	var pid int
	var process string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			value, err := strconv.Atoi(line[1:])
			if err != nil {
				return nil, fmt.Errorf("解析 lsof PID %q: %w", line, err)
			}
			pid = value
			process = ""
		case 'c':
			process = line[1:]
		case 'n':
			address := line[1:]
			port, ok := portFromAddress(address)
			if !ok || pid == 0 {
				continue
			}
			listeners = append(listeners, model.PortListener{
				Port:     port,
				PID:      pid,
				Process:  process,
				Address:  address,
				Protocol: "tcp",
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 lsof 输出: %w", err)
	}
	sort.Slice(listeners, func(i, j int) bool {
		if listeners[i].Port == listeners[j].Port {
			return listeners[i].PID < listeners[j].PID
		}
		return listeners[i].Port < listeners[j].Port
	})
	return listeners, nil
}

func portFromAddress(address string) (int, bool) {
	lastColon := strings.LastIndex(address, ":")
	if lastColon < 0 || lastColon == len(address)-1 {
		return 0, false
	}
	portText := address[lastColon+1:]
	if arrow := strings.Index(portText, "->"); arrow >= 0 {
		portText = portText[:arrow]
	}
	port, err := strconv.Atoi(portText)
	if err != nil || model.ValidatePort(port) != nil {
		return 0, false
	}
	return port, true
}

type Service struct {
	store          *store.Store
	scanner        Scanner
	now            func() time.Time
	observationMu  sync.Mutex
	observedAt     time.Time
	observed       []model.PortListener
	observationTTL time.Duration
}

type Availability struct {
	Port        int                    `json:"port"`
	Available   bool                   `json:"available"`
	Reason      string                 `json:"reason"`
	Listener    *model.PortListener    `json:"listener,omitempty"`
	Allocation  *PortAllocation        `json:"allocation,omitempty"`
	Reservation *model.PortReservation `json:"reservation,omitempty"`
}

type PortAllocation struct {
	Port        int                 `json:"port"`
	ProjectID   string              `json:"projectId"`
	ProjectName string              `json:"projectName"`
	State       string              `json:"state"`
	Listener    *model.PortListener `json:"listener,omitempty"`
}

type PoolSummary struct {
	Allocated           int `json:"allocated"`
	ActiveAllocations   int `json:"activeAllocations"`
	TemporaryReserved   int `json:"temporaryReserved"`
	UnassignedListeners int `json:"unassignedListeners"`
}

type PoolSnapshot struct {
	From                int                     `json:"from"`
	To                  int                     `json:"to"`
	Summary             PoolSummary             `json:"summary"`
	Allocations         []PortAllocation        `json:"allocations"`
	Reservations        []model.PortReservation `json:"reservations"`
	UnassignedListeners []model.PortListener    `json:"unassignedListeners"`
	Suggestions         []int                   `json:"suggestions"`
}

func NewService(store *store.Store, scanner Scanner) *Service {
	return &Service{
		store: store, scanner: scanner, now: time.Now,
		observationTTL: 2 * time.Second,
	}
}

// List always performs a fresh scan. Safety-sensitive checks, allocations, and
// lifecycle operations use this path so a recently bound socket is never hidden
// by the observation cache.
func (s *Service) List(ctx context.Context) ([]model.PortListener, error) {
	return s.scanner.List(ctx)
}

// Observe coalesces read-only UI observations for a short window. A dashboard
// refresh and a Widget refresh often arrive together; sharing that result avoids
// launching duplicate lsof processes without weakening port safety checks.
func (s *Service) Observe(ctx context.Context) ([]model.PortListener, error) {
	s.observationMu.Lock()
	defer s.observationMu.Unlock()
	if !s.observedAt.IsZero() && s.now().Sub(s.observedAt) < s.observationTTL {
		return cloneListeners(s.observed), nil
	}
	listeners, err := s.scanner.List(ctx)
	if err != nil {
		return nil, err
	}
	s.observed = cloneListeners(listeners)
	s.observedAt = s.now()
	return cloneListeners(listeners), nil
}

func (s *Service) Check(ctx context.Context, port int, forProject string) (Availability, error) {
	if err := model.ValidatePort(port); err != nil {
		return Availability{}, err
	}
	listeners, err := s.scanner.List(ctx)
	if err != nil {
		return Availability{}, err
	}
	for i := range listeners {
		if listeners[i].Port == port {
			return Availability{
				Port:      port,
				Available: false,
				Reason:    "端口已被真实进程监听",
				Listener:  &listeners[i],
			}, nil
		}
	}
	registry, err := s.store.Load(ctx)
	if err != nil {
		return Availability{}, err
	}
	now := s.now()
	var ownReservation *model.PortReservation
	for _, project := range registry.Projects {
		for _, allocatedPort := range project.Ports {
			if allocatedPort != port {
				continue
			}
			allocation := PortAllocation{
				Port: port, ProjectID: project.ID, ProjectName: project.Name, State: "idle",
			}
			if project.ID == forProject && forProject != "" {
				return Availability{
					Port: port, Available: true,
					Reason:     "端口已持久分配给当前项目，真实监听仍为空",
					Allocation: &allocation,
				}, nil
			}
			return Availability{
				Port: port, Available: false,
				Reason:     "端口已持久分配给其他项目",
				Allocation: &allocation,
			}, nil
		}
	}
	for i := range registry.Reservations {
		reservation := &registry.Reservations[i]
		if reservation.Port != port || !reservation.Active(now) {
			continue
		}
		if reservation.ProjectID == forProject && forProject != "" {
			copy := *reservation
			ownReservation = &copy
			continue
		}
		return Availability{
			Port:        port,
			Available:   false,
			Reason:      "端口已被其他项目预留",
			Reservation: reservation,
		}, nil
	}
	reason := "端口可用"
	if ownReservation != nil {
		reason = "端口已由当前项目预留，真实监听仍为空"
	}
	return Availability{Port: port, Available: true, Reason: reason, Reservation: ownReservation}, nil
}

func (s *Service) Reserve(ctx context.Context, reservation model.PortReservation) (model.PortReservation, error) {
	reservation.CreatedAt = s.now()
	if reservation.ExpiresAt.IsZero() {
		reservation.ExpiresAt = reservation.CreatedAt.Add(4 * time.Hour)
	}
	if err := reservation.Validate(); err != nil {
		return model.PortReservation{}, err
	}
	availability, err := s.Check(ctx, reservation.Port, reservation.ProjectID)
	if err != nil {
		return model.PortReservation{}, err
	}
	if availability.Allocation != nil {
		return model.PortReservation{}, fmt.Errorf("端口 %d 已持久分配给项目 %s，无需临时预留", reservation.Port, availability.Allocation.ProjectID)
	}
	if !availability.Available {
		return model.PortReservation{}, errors.New(availability.Reason)
	}
	_, err = s.store.Update(ctx, func(registry *model.Registry) error {
		now := s.now()
		for i := range registry.Reservations {
			existing := registry.Reservations[i]
			if existing.Port != reservation.Port || !existing.Active(now) {
				continue
			}
			if existing.ProjectID != reservation.ProjectID {
				return fmt.Errorf("端口 %d 已由项目 %s 预留", reservation.Port, existing.ProjectID)
			}
			registry.Reservations[i] = reservation
			return nil
		}
		registry.Reservations = append(registry.Reservations, reservation)
		return nil
	})
	if err != nil {
		return model.PortReservation{}, err
	}
	return reservation, nil
}

func (s *Service) ListAllocations(ctx context.Context) ([]PortAllocation, error) {
	listeners, err := s.scanner.List(ctx)
	if err != nil {
		return nil, err
	}
	return s.ListAllocationsWithListeners(ctx, listeners)
}

func (s *Service) ListAllocationsWithListeners(ctx context.Context, listeners []model.PortListener) ([]PortAllocation, error) {
	registry, err := s.store.Load(ctx)
	if err != nil {
		return nil, err
	}
	listenerByPort := map[int]model.PortListener{}
	for _, listener := range listeners {
		if _, exists := listenerByPort[listener.Port]; !exists {
			listenerByPort[listener.Port] = listener
		}
	}
	result := make([]PortAllocation, 0)
	for _, project := range registry.Projects {
		for _, port := range project.Ports {
			allocation := PortAllocation{
				Port: port, ProjectID: project.ID, ProjectName: project.Name, State: "idle",
			}
			if listener, exists := listenerByPort[port]; exists {
				copy := listener
				allocation.Listener = &copy
				allocation.State = "active"
			}
			result = append(result, allocation)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Port == result[j].Port {
			return result[i].ProjectName < result[j].ProjectName
		}
		return result[i].Port < result[j].Port
	})
	return result, nil
}

func (s *Service) Allocate(ctx context.Context, port int, projectID, owner string) (PortAllocation, error) {
	if err := model.ValidatePort(port); err != nil {
		return PortAllocation{}, err
	}
	owner = strings.ToLower(strings.TrimSpace(owner))
	if err := model.ValidatePortOwner(owner); err != nil {
		return PortAllocation{}, err
	}
	projectID = strings.ToLower(strings.TrimSpace(projectID))
	registry, err := s.store.Load(ctx)
	if err != nil {
		return PortAllocation{}, err
	}
	var existing *model.Project
	for i := range registry.Projects {
		if registry.Projects[i].ID == projectID {
			copy := registry.Projects[i]
			existing = &copy
			break
		}
	}
	if existing == nil {
		return PortAllocation{}, errors.New("项目不存在")
	}
	if slicesContain(existing.Ports, port) {
		allocations, listErr := s.ListAllocations(ctx)
		if listErr != nil {
			return PortAllocation{}, listErr
		}
		for _, allocation := range allocations {
			if allocation.Port == port && allocation.ProjectID == projectID {
				return allocation, nil
			}
		}
	}
	availability, err := s.Check(ctx, port, projectID)
	if err != nil {
		return PortAllocation{}, err
	}
	if !availability.Available {
		return PortAllocation{}, errors.New(availability.Reason)
	}
	_, err = s.store.Update(ctx, func(registry *model.Registry) error {
		var target *model.Project
		for i := range registry.Projects {
			project := &registry.Projects[i]
			if project.ID == projectID {
				target = project
			}
			for _, allocatedPort := range project.Ports {
				if allocatedPort == port && project.ID != projectID {
					return fmt.Errorf("端口 %d 已持久分配给项目 %s", port, project.ID)
				}
			}
		}
		if target == nil {
			return errors.New("项目不存在")
		}
		for _, reservation := range registry.Reservations {
			if reservation.Port == port && reservation.Active(s.now()) && reservation.ProjectID != projectID {
				return fmt.Errorf("端口 %d 已由项目 %s 临时预留", port, reservation.ProjectID)
			}
		}
		if !slicesContain(target.Ports, port) {
			target.Ports = append(target.Ports, port)
			sort.Ints(target.Ports)
			target.UpdatedAt = s.now()
		}
		filtered := registry.Reservations[:0]
		for _, reservation := range registry.Reservations {
			if reservation.Port == port && reservation.ProjectID == projectID {
				continue
			}
			filtered = append(filtered, reservation)
		}
		registry.Reservations = filtered
		return nil
	})
	if err != nil {
		return PortAllocation{}, err
	}
	return PortAllocation{
		Port: port, ProjectID: projectID, ProjectName: existing.Name, State: "idle",
	}, nil
}

func (s *Service) Unassign(ctx context.Context, port int, projectID string) error {
	if err := model.ValidatePort(port); err != nil {
		return err
	}
	_, err := s.store.Update(ctx, func(registry *model.Registry) error {
		for i := range registry.Projects {
			project := &registry.Projects[i]
			if project.ID != projectID {
				continue
			}
			filtered := make([]int, 0, len(project.Ports))
			found := false
			for _, allocatedPort := range project.Ports {
				if allocatedPort == port {
					found = true
					continue
				}
				filtered = append(filtered, allocatedPort)
			}
			if !found {
				return errors.New("没有找到匹配的持久端口分配")
			}
			project.Ports = filtered
			project.UpdatedAt = s.now()
			return nil
		}
		return errors.New("项目不存在")
	})
	return err
}

func (s *Service) ValidateAssignments(ctx context.Context, projectID string, proposed []int) error {
	registry, err := s.store.Load(ctx)
	if err != nil {
		return err
	}
	existingOwn := map[int]bool{}
	for _, project := range registry.Projects {
		if project.ID == projectID {
			for _, port := range project.Ports {
				existingOwn[port] = true
			}
		}
	}
	listeners, err := s.scanner.List(ctx)
	if err != nil {
		return err
	}
	listening := map[int]bool{}
	for _, listener := range listeners {
		listening[listener.Port] = true
	}
	now := s.now()
	for _, port := range proposed {
		if existingOwn[port] {
			continue
		}
		if listening[port] {
			return fmt.Errorf("端口 %d 已被真实进程监听", port)
		}
		for _, project := range registry.Projects {
			if project.ID != projectID && slicesContain(project.Ports, port) {
				return fmt.Errorf("端口 %d 已持久分配给项目 %s", port, project.ID)
			}
		}
		for _, reservation := range registry.Reservations {
			if reservation.Port == port && reservation.Active(now) && reservation.ProjectID != projectID {
				return fmt.Errorf("端口 %d 已由项目 %s 临时预留", port, reservation.ProjectID)
			}
		}
	}
	return nil
}

func (s *Service) Pool(ctx context.Context, from, to, limit int) (PoolSnapshot, error) {
	listeners, err := s.scanner.List(ctx)
	if err != nil {
		return PoolSnapshot{}, err
	}
	return s.PoolWithListeners(ctx, listeners, from, to, limit)
}

func (s *Service) PoolWithListeners(ctx context.Context, listeners []model.PortListener, from, to, limit int) (PoolSnapshot, error) {
	if from == 0 {
		from = 3000
	}
	if to == 0 {
		to = 49999
	}
	if err := model.ValidatePort(from); err != nil {
		return PoolSnapshot{}, err
	}
	if err := model.ValidatePort(to); err != nil || to < from {
		return PoolSnapshot{}, errors.New("端口池范围无效")
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	registry, err := s.store.Load(ctx)
	if err != nil {
		return PoolSnapshot{}, err
	}
	listenerByPort := map[int]model.PortListener{}
	for _, listener := range listeners {
		if _, exists := listenerByPort[listener.Port]; !exists {
			listenerByPort[listener.Port] = listener
		}
	}
	used := map[int]bool{}
	allocations := make([]PortAllocation, 0)
	activeCount := 0
	for _, project := range registry.Projects {
		for _, port := range project.Ports {
			used[port] = true
			allocation := PortAllocation{
				Port: port, ProjectID: project.ID, ProjectName: project.Name, State: "idle",
			}
			if listener, exists := listenerByPort[port]; exists {
				copy := listener
				allocation.Listener = &copy
				allocation.State = "active"
				activeCount++
			}
			allocations = append(allocations, allocation)
		}
	}
	now := s.now()
	reservations := make([]model.PortReservation, 0)
	for _, reservation := range registry.Reservations {
		if reservation.Active(now) {
			used[reservation.Port] = true
			reservations = append(reservations, reservation)
		}
	}
	unassigned := make([]model.PortListener, 0)
	for _, listener := range listenerByPort {
		usedByAllocation := false
		for _, allocation := range allocations {
			if allocation.Port == listener.Port {
				usedByAllocation = true
				break
			}
		}
		if !usedByAllocation {
			unassigned = append(unassigned, listener)
		}
		used[listener.Port] = true
	}
	suggestions := make([]int, 0, limit)
	for port := from; port <= to && len(suggestions) < limit; port++ {
		if !used[port] {
			suggestions = append(suggestions, port)
		}
	}
	sort.Slice(allocations, func(i, j int) bool { return allocations[i].Port < allocations[j].Port })
	sort.Slice(unassigned, func(i, j int) bool { return unassigned[i].Port < unassigned[j].Port })
	return PoolSnapshot{
		From: from, To: to,
		Summary: PoolSummary{
			Allocated: len(allocations), ActiveAllocations: activeCount,
			TemporaryReserved: len(reservations), UnassignedListeners: len(unassigned),
		},
		Allocations: allocations, Reservations: reservations,
		UnassignedListeners: unassigned, Suggestions: suggestions,
	}, nil
}

func cloneListeners(listeners []model.PortListener) []model.PortListener {
	return append([]model.PortListener(nil), listeners...)
}

func slicesContain(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (s *Service) Release(ctx context.Context, port int, projectID string) error {
	if err := model.ValidatePort(port); err != nil {
		return err
	}
	_, err := s.store.Update(ctx, func(registry *model.Registry) error {
		found := false
		filtered := registry.Reservations[:0]
		for _, reservation := range registry.Reservations {
			if reservation.Port == port && reservation.ProjectID == projectID {
				found = true
				continue
			}
			filtered = append(filtered, reservation)
		}
		registry.Reservations = filtered
		if !found {
			return errors.New("没有找到匹配的端口预留")
		}
		return nil
	})
	return err
}
