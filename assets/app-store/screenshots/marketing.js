const slides = [
  {
    image: "dashboard.png",
    eyebrow: "PROJECTDOCK",
    title: "一个窗口，掌控所有本地项目",
    subtitle: "状态、端口、运行控制与最近活动，一眼尽览。",
  },
  {
    image: "projects.png",
    eyebrow: "项目管理",
    title: "本地项目，一处归档管理",
    subtitle: "集中查看、整理并安全管理你的项目目录。",
  },
  {
    image: "local-scan.png",
    eyebrow: "本地发现",
    title: "扫描本地目录，发现可管理项目",
    subtitle: "授权目录后批量识别项目，快速纳入统一管理。",
  },
  {
    image: "api-workbench.png",
    eyebrow: "开发者工具",
    title: "内置安全的本地 API 工作台",
    subtitle: "查询接口、发送请求、检查响应，无需离开应用。",
  },
  {
    image: "audit.png",
    eyebrow: "操作审计",
    title: "每一步操作，都有清晰记录",
    subtitle: "项目操作与接口调用可追踪，排查问题更从容。",
  },
  {
    type: "languages",
    eyebrow: "全球化体验",
    title: "11 种语言，与系统无缝切换",
    subtitle: "支持简体中文、繁体中文及另外 9 种语言，包括英语、日语、韩语和阿拉伯语。",
  },
];

const index = Math.min(Math.max(Number(new URLSearchParams(location.search).get("slide") || 1), 1), slides.length) - 1;
const data = slides[index];
const root = document.querySelector("#slide");

const brand = `
  <div class="brand"><span class="brand-mark" aria-hidden="true"></span><span>ProjectDock</span></div>
`;

if (data.type === "languages") {
  root.classList.add("language-slide");
  root.innerHTML = `${brand}
    <section class="copy">
      <p class="eyebrow">${data.eyebrow}</p>
      <h1>${data.title}</h1>
      <p class="subtitle">${data.subtitle}</p>
      <div class="language-badges">
        <span>简体中文</span><span>繁體中文</span><span>English</span><span>日本語</span>
        <span>한국어</span><span>Español</span><span>Français</span><span>Deutsch</span>
        <span>Português</span><span>Русский</span><span>العربية</span>
      </div>
    </section>
    <div class="language-stack" aria-label="多语言界面预览">
      <div class="language-card"><img src="source/dashboard.png" alt="简体中文界面"></div>
      <div class="language-card"><img src="source/dashboard-en.png" alt="English interface"></div>
      <div class="language-card"><img src="source/dashboard-ar.png" alt="واجهة عربية"></div>
    </div>
    <div class="footer-note">真实应用界面 · macOS</div>`;
} else {
  root.innerHTML = `${brand}
    <section class="copy">
      <p class="eyebrow">${data.eyebrow}</p>
      <h1>${data.title}</h1>
      <p class="subtitle">${data.subtitle}</p>
    </section>
    <div class="screen-frame"><img src="source/${data.image}" alt="ProjectDock 应用界面"></div>
    <div class="footer-note">真实应用界面 · macOS</div>`;
}
