import { installLocaleDictionary, locales } from "./i18n";
import { localeDictionary } from "./i18n-catalogs";

for (const locale of locales) installLocaleDictionary(locale, localeDictionary(locale));

export * from "./i18n";
