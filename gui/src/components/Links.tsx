// Links.tsx — los enlaces a entidades. Donde la app nombra una cuenta, una
// conversación o una carpeta, el nombre lleva a su detalle: la cuenta a su
// espacio, la conversación a su panel, la carpeta al probador de Carpetas (que
// dice qué cuenta le toca y por qué regla).
//
// Son <a>, no <button>: la app corre en WebKit, y ahí el texto de un botón
// dentro de una celda estrecha se recortaba con «…» aunque hubiera sitio («a-…»
// en vez de «a-cc»). Un enlace es texto y se mide como texto.
//
// El nombre de una cuenta NUNCA se recorta: es corto y es lo que identifica la
// fila. Solo una ruta de carpeta, que puede ser larguísima, termina en «…».
//
// Paran la propagación del clic: muchos viven dentro de filas con su propio clic
// (un nodo del mapa, una fila que se arrastra), y el enlace no debe disparar
// además la acción de la fila.

import type { CSSProperties, MouseEvent, ReactNode } from 'react';
import { tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, type ProfileTab } from '../lib/store';

function act(fn: () => void) {
  return (e: MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    fn();
  };
}

export function AccountLink({ name, tab = 'resumen', swatch = true, style, children }: {
  name: string;
  tab?: ProfileTab;
  swatch?: boolean;
  style?: CSSProperties;
  children?: ReactNode;
}) {
  const { openProfile, colorOf, profiles } = useApp();
  // Una cuenta que ya no existe (una cadena que la nombra tras borrarla) no
  // lleva a ninguna parte: se enseña, pero no como enlace.
  const exists = name === 'default' || profiles.some((p) => p.name === name);
  const body = (
    <>
      {swatch && <span className="swatch" style={{ background: colorOf(name), width: 7, height: 7 }} />}
      {children ?? name}
    </>
  );
  if (!exists) return <span className="entity-link account off" style={style}>{body}</span>;
  return (
    <a
      href="#"
      className="entity-link account"
      style={style}
      title={t('Abrir la cuenta {p}', { p: name })}
      onPointerDown={(e) => e.stopPropagation()}
      onClick={act(() => openProfile(name, tab))}
    >
      {body}
    </a>
  );
}

export function ConvLink({ uuid, profile, children, style }: { uuid: string; profile: string; children: ReactNode; style?: CSSProperties }) {
  const { openConvPanel } = useApp();
  return (
    <a
      href="#"
      className="entity-link"
      style={style}
      title={t('Ver el detalle de la conversación')}
      onPointerDown={(e) => e.stopPropagation()}
      onClick={act(() => openConvPanel(uuid, profile))}
    >
      {children}
    </a>
  );
}

export function FolderLink({ path, style, children }: { path: string; style?: CSSProperties; children?: ReactNode }) {
  const { setFolder, go } = useApp();
  if (!path) return <span style={style}>—</span>;
  return (
    <a
      href="#"
      className="entity-link folder mono"
      style={style}
      title={path}
      onPointerDown={(e) => e.stopPropagation()}
      onClick={act(() => {
        setFolder(path);
        go('carpetas');
      })}
    >
      {children ?? tilde(path)}
    </a>
  );
}

/** Ancho en `ch` que necesita la cuenta de nombre más largo (más el color). Para
 *  las columnas «Cuenta» de las tablas: cada fila es su propia rejilla, así que
 *  el ancho tiene que ser el mismo para todas y caber el nombre entero. */
export function accountColumn(names: string[], min = 8): string {
  const n = Math.max(min, ...names.map((x) => x.length));
  return `calc(${n}ch + 22px)`;
}

/** Ancho en píxeles de un nodo de gráfico que tiene que caber el nombre entero
 *  (texto de ~12-13 px más el cuadrado de color y los márgenes). */
export function nodeWidth(names: string[], min: number, perChar = 7.4, extra = 44): number {
  return Math.max(min, Math.ceil(Math.max(0, ...names.map((x) => x.length)) * perChar + extra));
}
