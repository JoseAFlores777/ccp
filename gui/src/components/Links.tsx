// Links.tsx — los enlaces a entidades. Donde la app nombra una cuenta, una
// conversación o una carpeta, el nombre lleva a su detalle: la cuenta a su
// espacio, la conversación a su panel, la carpeta al probador de Carpetas (que
// dice qué cuenta le toca y por qué regla).
//
// Paran la propagación del clic: muchos aparecen dentro de filas que ya tienen
// su propio clic (un nodo del mapa, una fila que se arrastra), y el enlace no
// debe disparar además la acción de la fila.

import type { CSSProperties, ReactNode } from 'react';
import { tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, type ProfileTab } from '../lib/store';

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
  if (!exists) return <span className="entity-link off" style={style}>{body}</span>;
  return (
    <button
      type="button"
      className="entity-link"
      style={style}
      title={t('Abrir la cuenta {p}', { p: name })}
      onPointerDown={(e) => e.stopPropagation()}
      onClick={(e) => {
        e.stopPropagation();
        openProfile(name, tab);
      }}
    >
      {body}
    </button>
  );
}

export function ConvLink({ uuid, profile, children, style }: { uuid: string; profile: string; children: ReactNode; style?: CSSProperties }) {
  const { openConvPanel } = useApp();
  return (
    <button
      type="button"
      className="entity-link"
      style={style}
      title={t('Ver el detalle de la conversación')}
      onPointerDown={(e) => e.stopPropagation()}
      onClick={(e) => {
        e.stopPropagation();
        openConvPanel(uuid, profile);
      }}
    >
      {children}
    </button>
  );
}

export function FolderLink({ path, style, children }: { path: string; style?: CSSProperties; children?: ReactNode }) {
  const { setFolder, go } = useApp();
  if (!path) return <span style={style}>—</span>;
  return (
    <button
      type="button"
      className="entity-link mono"
      style={style}
      title={t('Ver qué cuenta usa esta carpeta y por qué')}
      onPointerDown={(e) => e.stopPropagation()}
      onClick={(e) => {
        e.stopPropagation();
        setFolder(path);
        go('carpetas');
      }}
    >
      {children ?? tilde(path)}
    </button>
  );
}
