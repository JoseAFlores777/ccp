// App.tsx — enruta la pantalla elegida dentro del marco y monta los
// elementos flotantes (modal, hoja de terminal, aviso).

import { Modal } from './components/Modal';
import { TerminalSheet, ToastView } from './components/Overlays';
import { Header, Shell } from './components/Shell';
import { bridgeInfo } from './lib/bridge';
import { t } from './lib/i18n';
import { useApp, type Screen } from './lib/store';
import { Detectar } from './screens/Detectar';
import { Ajustes, Copias } from './screens/Ajustes';
import { Carpetas } from './screens/Carpetas';
import { Config } from './screens/Config';
import { Configuracion } from './screens/Configuracion';
import { Conversaciones } from './screens/Conversaciones';
import { Desktop } from './screens/Desktop';
import { Diagnostico } from './screens/Diagnostico';
import { Inicio } from './screens/Inicio';
import { Mapa } from './screens/Mapa';
import { Memoria } from './screens/Memoria';
import { Mover } from './screens/Mover';
import { Nube } from './screens/Nube';
import { Perfil } from './screens/Perfil';
import { Perfiles } from './screens/Perfiles';
import { Prestamos } from './screens/Prestamos';
import { Rotacion } from './screens/Rotacion';
import { Sesiones } from './screens/Sesiones';
import { Snapshots } from './screens/Snapshots';
import { Uso } from './screens/Uso';

const SCREENS: Record<Screen, () => React.JSX.Element | null> = {
  inicio: Inicio,
  mapa: Mapa,
  perfiles: Perfiles,
  perfil: Perfil,
  config: Config,
  configuracion: Configuracion,
  carpetas: Carpetas,
  conv: Conversaciones,
  mover: Mover,
  prestamos: Prestamos,
  rotacion: Rotacion,
  uso: Uso,
  sesiones: Sesiones,
  desktop: Desktop,
  diag: Diagnostico,
  memoria: Memoria,
  ajustes: Ajustes,
  copias: Copias,
  snapshots: Snapshots,
  nube: Nube,
  bienvenida: Detectar,
};

/** Si el motor no arranca no hay nada que enseñar: se dice qué falló y cómo
 *  seguir, en vez de pintar pantallas vacías. */
function EngineDown({ error }: { error: string }) {
  return (
    <div style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 30, background: 'var(--bg)' }} data-tauri-drag-region>
      <div className="card pad shadow" style={{ maxWidth: 560 }}>
        <div className="label" style={{ color: 'var(--err)', marginBottom: 10 }}>{t('No se pudo hablar con ccp')}</div>
        <div style={{ fontSize: 13, color: 'var(--ink-2)', lineHeight: 1.6, fontWeight: 300 }}>
          {t('La app necesita un ccp que traiga el comando «serve». Si el tuyo es más viejo, actualízalo con ccp upgrade; si no tienes ninguno, instálalo con el instalador del README.')}
        </div>
        <pre className="mono selectable" style={{ marginTop: 14, fontSize: 11, whiteSpace: 'pre-wrap', color: 'var(--err)', background: 'var(--err-soft)', padding: 12, borderRadius: 8 }}>{error}</pre>
        <button className="btn lg" style={{ marginTop: 14 }} onClick={() => window.location.reload()}>{t('Reintentar')}</button>
        <button className="btn lg ghost" style={{ marginTop: 14, marginLeft: 8 }} onClick={() => void bridgeInfo().then((b) => alert(JSON.stringify(b, null, 2)))}>
          {t('Ver qué binario se usa')}
        </button>
      </div>
    </div>
  );
}

export function App() {
  const { screen, infoError, info } = useApp();
  if (infoError && !info) return <EngineDown error={infoError} />;
  const View = SCREENS[screen] ?? Inicio;
  return (
    <Shell>
      <Header />
      {info ? <View key={screen} /> : <div className="skeleton" style={{ height: 200 }} />}
      <Modal />
      <TerminalSheet />
      <ToastView />
    </Shell>
  );
}
