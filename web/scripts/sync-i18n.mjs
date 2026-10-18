// The web catalogs are the source of truth for marked system copy. Go embeds
// the same messages for API errors and request-triggered verification emails.
// Run `npm run i18n:sync` after changing catalogs; `npm test` checks for drift.
import { readFile, writeFile } from "node:fs/promises";
const root = new URL("../", import.meta.url);
const messages = {};
for (const name of ["common", "customer", "operator", "forms", "backend", "errors", "help", "customer-ux", "operator-ux", "discovery-ux"]) {
  Object.assign(messages, JSON.parse(await readFile(new URL(`locales/${name}.json`, root), "utf8")));
}
const generated = Object.fromEntries(Object.keys(messages).sort().map(source => {
  const value = messages[source];
  for (const locale of ["en", "ja", "ru"]) {
    if (typeof value[locale] !== "string" || !value[locale].trim()) throw new Error(`Missing ${locale}: ${source}`);
  }
  return [source, [value.zh || source, value.en, value.ja, value.ru]];
}));
const target = new URL("../backend/internal/platform/httpx/ui_i18n.generated.json", root);
const content = JSON.stringify(generated, null, 2) + "\n";
if (process.argv.includes("--check")) {
  if (await readFile(target, "utf8") !== content) throw new Error("Backend translation catalog is stale. Run npm run i18n:sync.");
} else {
  await writeFile(target, content);
}
