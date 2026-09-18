// i18n.ts — la interfaz habla español o inglés, como el resto de ccp.
//
// El texto fuente es el español del diseño: t('Nueva cuenta') devuelve la
// frase en español o su traducción inglesa si el idioma es `en`. Una frase sin
// traducir cae al español en vez de enseñar una clave cruda. Las variables van
// entre llaves: t('Se reinicia a las {h}', { h: '14:30' }).
//
// Los nombres de cuentas, carpetas, comandos y códigos de diagnóstico nunca se
// traducen: son datos, no texto.

import { en } from './i18n_en';

export type Lang = 'es' | 'en';

let current: Lang = 'es';

export function setLang(l: Lang) {
  current = l;
  document.documentElement.lang = l;
}

export function getLang(): Lang {
  return current;
}

export function t(s: string, vars?: Record<string, string | number>): string {
  let out = current === 'en' ? en[s] ?? s : s;
  if (vars) out = out.replace(/\{(\w+)\}/g, (_, k: string) => (k in vars ? String(vars[k]) : `{${k}}`));
  return out;
}
