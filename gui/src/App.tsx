// App.tsx — enruta la pantalla elegida dentro del marco y monta los
// elementos flotantes (modal, hoja de terminal, aviso).

import { Boundary } from './components/Boundary';
import { Modal } from './components/Modal';
import { TerminalSheet, ToastView } from './components/Overlays';
import { Header, Shell } from './components/Shell';
import { bridgeInfo } from './lib/bridge';
import { t } from './lib/i18n';
import { useEffect } from 'react';
import { useApp, type Screen } from './lib/store';
import { resumeAfterUpgrade } from './lib/upgrade';
import { Detectar } from './screens/Detectar';
import { Glosario } from './screens/Glosario';
import { Ajustes, Copias } from './screens/Ajustes';
import { Carpetas } from './screens/Carpetas';
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
import { Rotacion } from './screens/Rotacion';
import { Sesiones } from './screens/Sesiones';
import { Snapshots } from './screens/Snapshots';
import { Uso } from './screens/Uso';

const SCREENS: Record<Screen, () => React.JSX.Element | null> = {
  inicio: Inicio,
  mapa: Mapa,
  perfiles: Perfiles,
  perfil: Perfil,
  configuracion: Configuracion,
  carpetas: Carpetas,
  conv: Conversaciones,
  mover: Mover,
  // Los préstamos son una vista de Conversaciones: la ruta se conserva para
  // que los enlaces a «Préstamos» sigan llegando.
  prestamos: () => <Conversaciones view="loans" />,
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
  glosario: Glosario,
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
  const app = useApp();
  const { screen, infoError, info } = app;
  // Tras «Actualizar ccp» la app se reinicia sola; aquí, ya con el motor nuevo
  // respondiendo, se reinician las ventanas de Desktop que quedaron apuntadas.
  const ready = !!info;
  useEffect(() => {
    if (ready) void resumeAfterUpgrade(app);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready]);
  if (infoError && !info) return <EngineDown error={infoError} />;
  const View = SCREENS[screen] ?? Inicio;
  return (
    <Shell>
      <Header />
      {/* Boundary por PANTALLA y otro para el modal, no uno solo arriba: así un
          fallo en una vista deja la barra lateral viva y se puede ir a otra, y un
          modal roto no se lleva por delante la pantalla que hay detrás. La `key`
          lo remonta al cambiar de pantalla, que es lo que limpia el estado de
          error sin recargar la app. */}
      <Boundary key={screen}>
        {info ? <View /> : <div className="skeleton" style={{ height: 200 }} />}
      </Boundary>
      <Boundary>
        <Modal />
      </Boundary>
      <TerminalSheet />
      <ToastView />
    </Shell>
  );
}
