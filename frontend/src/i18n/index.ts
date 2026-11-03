import dayjs from "dayjs";
import { createI18n } from "vue-i18n";

import english from "./en.json";
const messages = { en: english };
const translations = import.meta.glob(["./*.json", "!./en.json"], {
  import: "default",
});
const dayLocales = {
  ar: () => import("dayjs/locale/ar"),
  bg: () => import("dayjs/locale/bg"),
  ca: () => import("dayjs/locale/ca"),
  cs: () => import("dayjs/locale/cs"),
  de: () => import("dayjs/locale/de"),
  el: () => import("dayjs/locale/el"),
  en: () => import("dayjs/locale/en"),
  es: () => import("dayjs/locale/es"),
  fr: () => import("dayjs/locale/fr"),
  he: () => import("dayjs/locale/he"),
  hr: () => import("dayjs/locale/hr"),
  hu: () => import("dayjs/locale/hu"),
  is: () => import("dayjs/locale/is"),
  it: () => import("dayjs/locale/it"),
  ja: () => import("dayjs/locale/ja"),
  ko: () => import("dayjs/locale/ko"),
  lv: () => import("dayjs/locale/lv"),
  nb: () => import("dayjs/locale/nb"),
  nl: () => import("dayjs/locale/nl"),
  "nl-be": () => import("dayjs/locale/nl-be"),
  pl: () => import("dayjs/locale/pl"),
  "pt-br": () => import("dayjs/locale/pt-br"),
  pt: () => import("dayjs/locale/pt"),
  ro: () => import("dayjs/locale/ro"),
  ru: () => import("dayjs/locale/ru"),
  sk: () => import("dayjs/locale/sk"),
  sv: () => import("dayjs/locale/sv"),
  tr: () => import("dayjs/locale/tr"),
  uk: () => import("dayjs/locale/uk"),
  vi: () => import("dayjs/locale/vi"),
  "zh-cn": () => import("dayjs/locale/zh-cn"),
  "zh-tw": () => import("dayjs/locale/zh-tw"),
} as Record<string, () => Promise<unknown>>;
let localeRequest = 0;
export function detectLocale() {
  // locale is an RFC 5646 language tag
  // https://developer.mozilla.org/en-US/docs/Web/API/Navigator/language
  let locale = navigator.language.toLowerCase();
  switch (true) {
    case /^ar\b/.test(locale):
      locale = "ar";
      break;
    case /^bg\b/.test(locale):
      locale = "bg";
      break;
    case /^cs\b/.test(locale):
      locale = "cs";
      break;
    case /^lv\b/.test(locale):
      locale = "lv";
      break;
    case /^he\b/.test(locale):
      locale = "he";
      break;
    case /^hr\b/.test(locale):
      locale = "hr";
      break;
    case /^hu\b/.test(locale):
      locale = "hu";
      break;
    case /^el.*/i.test(locale):
      locale = "el";
      break;
    case /^es\b/.test(locale):
      locale = "es";
      break;
    case /^en\b/.test(locale):
      locale = "en";
      break;
    case /^is\b/.test(locale):
      locale = "is";
      break;
    case /^it\b/.test(locale):
      locale = "it";
      break;
    case /^fr\b/.test(locale):
      locale = "fr";
      break;
    case /^pt-br\b/.test(locale):
      locale = "pt-br";
      break;
    case /^pt-pt\b/.test(locale):
    case /^pt\b/.test(locale):
      locale = "pt-pt";
      break;
    case /^ja\b/.test(locale):
      locale = "ja";
      break;
    case /^zh-tw\b/.test(locale):
      locale = "zh-tw";
      break;
    case /^zh-cn\b/.test(locale):
    case /^zh\b/.test(locale):
      locale = "zh-cn";
      break;
    case /^de\b/.test(locale):
      locale = "de";
      break;
    case /^ro\b/.test(locale):
      locale = "ro";
      break;
    case /^ru\b/.test(locale):
      locale = "ru";
      break;
    case /^pl\b/.test(locale):
      locale = "pl";
      break;
    case /^ko\b/.test(locale):
      locale = "ko";
      break;
    case /^sk\b/.test(locale):
      locale = "sk";
      break;
    case /^tr\b/.test(locale):
      locale = "tr";
      break;
    case /^uk\b/.test(locale):
      locale = "uk";
      break;
    case /^vi\b/.test(locale):
      locale = "vi";
      break;
    case /^sv-se\b/.test(locale):
    case /^sv\b/.test(locale):
      locale = "sv";
      break;
    case /^nl\b/.test(locale):
      locale = "nl";
      break;
    case /^nl-be\b/.test(locale):
      locale = "nl-be";
      break;
    case /^nb\b/.test(locale):
    case /^no\b/.test(locale):
      locale = "no";
      break;

    default:
      locale = "en";
  }

  return locale;
}

// TODO: was this really necessary?
// function removeEmpty(obj: Record<string, any>): void {
//   Object.keys(obj)
//     .filter((k) => obj[k] !== null && obj[k] !== undefined && obj[k] !== "") // Remove undef. and null and empty.string.
//     .reduce(
//       (newObj, k) =>
//         typeof obj[k] === "object"
//           ? Object.assign(newObj, { [k]: removeEmpty(obj[k]) }) // Recurse.
//           : Object.assign(newObj, { [k]: obj[k] }), // Copy value.
//       {}
//     );
// }

export const rtlLanguages = ["he", "ar"];

export const i18n = createI18n({
  locale: "en",
  fallbackLocale: "en",
  messages,
  // expose i18n.global for outside components
  legacy: true,
});

export const isRtl = (locale?: string) => {
  // see below
  // @ts-expect-error incorrect type when legacy
  return rtlLanguages.includes(locale || i18n.global.locale.value);
};

export async function setLocale(locale: string) {
  const request = ++localeRequest;
  const load = translations[`./${locale}.json`];
  if (locale !== "en" && load) {
    i18n.global.setLocaleMessage(locale, (await load()) as any);
  } else if (locale !== "en") {
    locale = "en";
  }
  const dayLocale = locale === "pt-pt" ? "pt" : locale === "no" ? "nb" : locale;
  await dayLocales[dayLocale]?.();
  if (request !== localeRequest) return;
  dayjs.locale(dayLocale);
  // @ts-expect-error legacy typings do not expose the composition locale ref
  i18n.global.locale.value = locale;
  setHtmlLocale(locale);
}
void setLocale(detectLocale());

export function setHtmlLocale(locale: string) {
  if (typeof document === "undefined") return;
  const html = document.documentElement;
  html.lang = locale;
  if (isRtl(locale)) html.dir = "rtl";
  else html.dir = "ltr";
}

export default i18n;
