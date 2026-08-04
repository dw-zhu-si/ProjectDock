import assert from "node:assert/strict";

const storage = new Map();
globalThis.localStorage = {
  getItem: (key) => storage.get(key) ?? null,
  setItem: (key, value) => storage.set(key, String(value)),
};
Object.defineProperty(globalThis, "navigator", { value: { language: "en-US", languages: ["en-US"] } });

const { localeCoverage, setLocalePreference, supportedLocales, t } = await import("../web/js/i18n.js");
const locales = supportedLocales.map(({ code }) => code);
assert.equal(new Set(locales).size, locales.length, "locale codes must be unique");
assert.equal(locales.length, 12, "auto plus 11 explicit locales are required");

for (const [locale, result] of Object.entries(localeCoverage())) {
  assert.ok(result.expected >= 100, `${locale} catalog is unexpectedly small`);
  assert.equal(result.complete, true, `${locale} catalog is incomplete`);
  assert.equal(result.translated, result.expected, `${locale} has missing rows`);
  assert.equal(setLocalePreference(locale), true);
  assert.notEqual(t("运行总览"), "", `${locale} overview translation is empty`);
  assert.notEqual(t("彻底卸载并删除全部内容"), "", `${locale} destructive action translation is empty`);
}

setLocalePreference("ar");
assert.equal(t("总览"), "نظرة عامة");
setLocalePreference("zh-Hant");
assert.equal(t("项目管理"), "專案管理");
setLocalePreference("en");
assert.equal(t("AI 设置"), "AI settings");

console.log(JSON.stringify(localeCoverage(), null, 2));
