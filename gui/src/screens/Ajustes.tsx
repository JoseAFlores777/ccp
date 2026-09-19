// P-16 Ajustes y P-17 Copias de seguridad — lo que se configura una vez y
// se revisa rara vez.

import { useState, type ReactNode } from 'react';
import { api, type CliRun, type ConfigInfo, type RestoreReport } from '../lib/api';
import { backupName, EFFORTS, must } from '../lib/actions';
import { pickOpenFile, pickSaveFile, revealPath } from '../lib/bridge';
import { tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall, type ModalSpec } from '../lib/store';
import { Card, Checkbox, CliBar, CommandOutput, KV, Label, Note, Segmented } from '../components/ui';

function SettingRow({ label, desc, value, children }: { label: string; desc: ReactNode; value?: ReactNode; children?: ReactNode }) {
  return (
    <Card style={{ padding: '16px 20px', display: 'flex', gap: 16, alignItems: 'center' }}>
      <span style={{ flex: 1, minWidth: 0 }}>
        <span style={{ display: 'block', fontSize: 13, color: 'var(--ink)' }}>{label}</span>
        <span style={{ display: 'block', fontSize: 12, color: 'var(--ink-3)', marginTop: 4, lineHeight: 1.55, fontWeight: 300 }}>{desc}</span>
      </span>
      {value != null && <span className="mono" style={{ fontSize: 11.5, color: 'var(--ink-3)', flex: '0 0 auto', maxWidth: 260, textAlign: 'right' }}>{value}</span>}
      <span style={{ display: 'flex', gap: 7, flex: '0 0 auto' }}>{children}</span>
    </Card>
  );
}

function defaultsModal(cfg: ConfigInfo): ModalSpec {
  const d = cfg.defaults;
  return {
    title: t('Valores para perfiles DeepSeek nuevos'),
    sub: t('Solo siembran los perfiles que se creen a partir de ahora. Los que ya existen no cambian: no hay herencia.'),
    initial: { ...d },
    fields: [
      { key: 'base_url', label: t('Endpoint'), kind: 'text' },
      { key: 'model_pro', label: t('Modelo pro'), kind: 'text' },
      { key: 'model_flash', label: t('Modelo flash'), kind: 'text' },
      { key: 'effort', label: t('Esfuerzo'), kind: 'select', options: (EFFORTS.includes(d.effort) ? EFFORTS : [...EFFORTS, d.effort]).map((e) => ({ value: e, label: e })) },
    ],
    canConfirm: (f) => ['base_url', 'model_pro', 'model_flash', 'effort'].every((k) => (f[k] ?? '').trim()),
    confirmLabel: t('Guardar'),
    cli: (f) => ['base_url', 'model_pro', 'model_flash', 'effort'].filter((k) => f[k] !== (d as Record<string, string>)[k]).map((k) => `ccp config set ${k} ${f[k]}`).join(' && ') || 'ccp config show',
    onConfirm: async (f) => {
      for (const k of ['base_url', 'model_pro', 'model_flash', 'effort']) {
        if (f[k].trim() !== (d as Record<string, string>)[k]) await api.setDefault(k, f[k].trim());
      }
      return t('Valores por defecto guardados');
    },
    undo: () => async () => {
      for (const k of ['base_url', 'model_pro', 'model_flash', 'effort']) await api.setDefault(k, (d as Record<string, string>)[k]);
    },
  };
}

function editorModal(cfg: ConfigInfo): ModalSpec {
  return {
    title: t('Editores'),
    sub: t('El de terminal se usa en ccp path edit y ccp profile config; el gráfico, en ccp config edit. Vacío = $EDITOR.'),
    initial: { editor: cfg.editor, gui_editor: cfg.gui_editor },
    fields: [
      { key: 'editor', label: t('Editor de terminal'), kind: 'text', placeholder: 'vim' },
      { key: 'gui_editor', label: t('Editor gráfico'), kind: 'text', placeholder: 'code --wait' },
    ],
    confirmLabel: t('Guardar'),
    cli: (f) => `ccp config editor ${f.editor || '""'} && ccp config gui-editor ${f.gui_editor || '""'}`,
    onConfirm: async (f) => {
      await api.setEditor({ editor: f.editor ?? '', gui_editor: f.gui_editor ?? '' });
      return t('Editores guardados');
    },
  };
}

function upgradeModal(onRun: (r: CliRun) => void): ModalSpec {
  return {
    title: t('Actualizar ccp'),
    initial: { source: 'release' },
    fields: [
      {
        key: 'source', label: t('De dónde'), kind: 'select',
        options: [
          { value: 'release', label: t('La última versión publicada (recomendado)') },
          { value: 'source', label: t('Compilar el repo registrado (necesita Go)') },
          { value: 'pull', label: t('Actualizar el repo con git pull y compilarlo') },
        ],
      },
    ],
    warns: [
      t('Vuelve a ejecutar el instalador y después resincroniza todos los perfiles.'),
      t('Los lanzadores de Desktop se ponen al día en su siguiente arranque.'),
      t('La app sigue usando el ccp con el que arrancó hasta que la reinicies.'),
    ],
    confirmLabel: t('Actualizar'),
    cli: (f) => ['ccp upgrade', f.source === 'source' ? '--from-source' : '', f.source === 'pull' ? '--from-source --pull' : ''].filter(Boolean).join(' '),
    onConfirm: async (f) => {
      const r = await api.system('upgrade', f.source !== 'release', f.source === 'pull');
      onRun(r);
      must(r);
      return t('ccp actualizado. Reinicia la app para usar la versión nueva.');
    },
  };
}

function uninstallModal(onRun: (r: CliRun) => void): ModalSpec {
  return {
    title: t('Quitar la integración con la shell'),
    sub: t('Es reversible: ccp install la vuelve a poner.'),
    warns: [
      t('Se quita el bloque de ccp del rc. Las terminales nuevas dejan de cambiar de cuenta al cambiar de carpeta.'),
      t('No se borra ninguna cuenta, regla ni conversación: ~/.config/ccp queda intacto.'),
      t('Borrar también las cuentas y la configuración es otra decisión y se hace a mano, después de exportar una copia.'),
    ],
    danger: true,
    confirmLabel: t('Quitar la integración'),
    cli: () => 'ccp uninstall',
    onConfirm: async () => {
      const r = await api.system('uninstall');
      onRun(r);
      must(r);
      return t('Integración quitada. ccp install la devuelve.');
    },
  };
}

export function Ajustes() {
  const app = useApp();
  const { info, lang, setLang, openModal, go, bridge, mutate } = app;
  const cfg = useCall(() => api.config(), []);
  const [run, setRun] = useState<CliRun | null>(null);

  const shell = info?.shell;
  const langSource = info?.lang_source === 'env' ? t('por CCP_LANG') : info?.lang_source === 'config' ? t('guardado en ccp.yaml') : t('por defecto');

  return (
    <div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        <SettingRow
          label={t('Idioma')}
          desc={t('Se cambia al momento y es el mismo de la CLI. Los nombres de cuentas, carpetas y comandos no se traducen.')}
          value={`${lang === 'es' ? 'Español' : 'English'} · ${langSource}`}
        >
          <Segmented value={lang} onChange={(l) => void setLang(l)} options={[{ value: 'es', label: 'Español' }, { value: 'en', label: 'English' }]} />
        </SettingRow>

        <SettingRow
          label={t('Valores por defecto para perfiles DeepSeek nuevos')}
          desc={t('Endpoint, modelos pro y flash, esfuerzo. Kimi y GLM usan los de su proveedor. Los perfiles que ya existen no cambian: no hay herencia.')}
          value={cfg.data ? cfg.data.defaults.model_pro : ''}
        >
          <button className="btn" disabled={!cfg.data} onClick={() => cfg.data && openModal(defaultsModal(cfg.data))}>{t('Editar')}</button>
          <button className="btn ghost" onClick={() => mutate(() => api.resetDefaults(), { msg: t('Valores por defecto restablecidos') })}>{t('Restablecer')}</button>
        </SettingRow>

        <SettingRow
          label={t('Editores')}
          desc={t('Con qué se abren ccp.yaml, las reglas y el overlay de un perfil cuando se editan a mano.')}
          value={cfg.data ? [cfg.data.editor || '$EDITOR', cfg.data.gui_editor].filter(Boolean).join(' · ') : ''}
        >
          <button className="btn" disabled={!cfg.data} onClick={() => cfg.data && openModal(editorModal(cfg.data))}>{t('Editar')}</button>
        </SettingRow>

        <SettingRow
          label={t('Integración con la shell')}
          desc={
            !shell?.installed
              ? t('No está instalada en {rc}: cambiar de carpeta no cambia de cuenta en la terminal.', { rc: tilde(shell?.rc) })
              : shell.stale
                ? t('El bloque de {rc} es de una versión anterior: los subcomandos nuevos pueden fallar.', { rc: tilde(shell.rc) })
                : t('Instalada y al día en {rc}.', { rc: tilde(shell.rc) })
          }
          value={!shell?.installed ? t('No instalada') : shell.stale ? t('Desfasada') : t('Al día')}
        >
          <button
            className={`btn ${!shell?.installed || shell?.stale ? 'primary' : ''}`}
            onClick={() => mutate(async () => must(await api.system('install')), { msg: t('Integración con la shell al día') })}
          >
            {!shell?.installed ? t('Instalar') : shell.stale ? t('Refrescar') : t('Reinstalar')}
          </button>
        </SettingRow>

        <SettingRow
          label={t('Actualizar ccp')}
          desc={t('Después resincroniza los perfiles; los lanzadores se ponen al día en su siguiente arranque.')}
          value={info ? `v${info.version.replace(/^v/, '')}` : ''}
        >
          <button className="btn" onClick={() => openModal(upgradeModal(setRun))}>{t('Actualizar')}</button>
        </SettingRow>

        <SettingRow label={t('Copias de seguridad')} desc={t('Exportar con o sin secretos, y restaurar sabiendo qué se escribe.')}>
          <button className="btn" onClick={() => go('copias')}>{t('Abrir')}</button>
        </SettingRow>

        <SettingRow label={t('Detectar esta máquina')} desc={t('Todo lo de Claude que hay aquí y el plan para traer a ccp lo que vive fuera.')}>
          <button className="btn" onClick={() => go('bienvenida')}>{t('Abrir')}</button>
        </SettingRow>

        <SettingRow
          label={t('Desinstalar')}
          desc={t('Quitar la integración es reversible. Borrar también cuentas y configuración no se hace desde aquí: pide exportar una copia antes y hacerlo a mano.')}
          value={t('dos niveles')}
        >
          <button className="btn danger" onClick={() => openModal(uninstallModal(setRun))}>{t('Desinstalar…')}</button>
        </SettingRow>
      </div>

      {run && (
        <div style={{ marginTop: 14 }}>
          <CommandOutput run={run} />
        </div>
      )}

      <Card style={{ marginTop: 14 }}>
        <Label style={{ marginBottom: 12 }}>{t('Acerca de esta app')}</Label>
        <KV
          labelWidth={150}
          rows={[
            [t('Versión de ccp'), info?.version ?? '—'],
            [t('Configuración'), tilde(info?.home) || '—'],
            [t('Motor'), bridge ? `${tilde(bridge.binary)} (${bridge.source === 'bundled' ? t('incluido en la app') : bridge.source === 'installed' ? t('el instalado') : bridge.source === 'env' ? 'CCP_GUI_BIN' : t('desarrollo')})` : '—'],
            [t('Sensores apuntan a'), tilde(info?.sensor_bin) || '—'],
          ]}
        />
        {bridge?.fallback_reason && <Note kind="warn" style={{ marginTop: 12 }}>{bridge.fallback_reason}</Note>}
        {info?.home && (
          <button className="btn ghost sm" style={{ marginTop: 12 }} onClick={() => void revealPath(info.home)}>
            {t('Mostrar en Finder')}
          </button>
        )}
      </Card>
      <CliBar cmd="ccp config show && ccp lang" />
    </div>
  );
}

export function Copias() {
  const app = useApp();
  const { info, openModal, go } = app;
  const [withSecrets, setWithSecrets] = useState(false);
  const [report, setReport] = useState<RestoreReport | null>(null);
  const home = info?.user_home ?? '';

  const exportNow = async () => {
    const dest = await pickSaveFile(`${home}/${backupName()}`);
    if (!dest) return;
    await app.mutate(() => api.backupExport(dest, withSecrets), {
      msg: withSecrets ? t('Copia con secretos guardada en {f}. Guárdala como una contraseña.', { f: tilde(dest) }) : t('Copia guardada en {f}', { f: tilde(dest) }),
    });
  };

  const restore = async () => {
    const archive = await pickOpenFile();
    if (!archive) return;
    openModal({
      title: t('Restaurar {f}', { f: tilde(archive) }),
      sub: t('Antes de escribir nada, ccp guarda una foto completa de ccp.yaml y de profiles/, así que se puede volver atrás.'),
      initial: { mode: 'merge', confirm: '' },
      fields: [
        {
          key: 'mode', label: t('Qué hacer con lo que ya existe'), kind: 'select',
          options: [
            { value: 'merge', label: t('Fusionar: añadir lo que falta y no tocar lo que ya hay') },
            { value: 'overwrite', label: t('Sobrescribir los perfiles que vengan en la copia') },
            { value: 'force', label: t('Reemplazar todo por lo que trae la copia') },
          ],
        },
        { key: 'confirm', label: t('Escribe {w} para confirmar', { w: t('reemplazar') }), kind: 'text', show: (f) => f.mode === 'force' },
      ],
      warns: (f) =>
        f.mode === 'merge'
          ? [t('Los perfiles que ya existan se saltan; las reglas nuevas se añaden.')]
          : f.mode === 'overwrite'
            ? [t('Los perfiles que vengan en la copia sustituyen a los tuyos con el mismo nombre, key incluida si la trae.')]
            : [
                t('Borra ccp.yaml y TODOS los perfiles, con sus conversaciones y sus ventanas de Desktop, y deja solo lo que trae la copia.'),
                t('La foto previa permite recuperarlo, pero ocupa lo mismo que tus perfiles.'),
              ],
      canConfirm: (f) => f.mode !== 'force' || (f.confirm ?? '').trim() === t('reemplazar'),
      confirmLabel: t('Restaurar'),
      cli: (f) => `ccp backup restore ${tilde(archive)}${f.mode === 'overwrite' ? ' --overwrite' : f.mode === 'force' ? ' --force' : ''}`,
      onConfirm: async (f) => {
        const r = await api.backupRestore(archive, f.mode as 'merge' | 'overwrite' | 'force');
        setReport(r);
        return t('Copia restaurada');
      },
    });
  };

  return (
    <div>
      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0,1fr) minmax(0,1fr)', gap: 14 }}>
        <Card shadow>
          <Label style={{ marginBottom: 12 }}>{t('Exportar')}</Label>
          <div style={{ fontSize: 12.5, color: 'var(--ink-3)', lineHeight: 1.6, fontWeight: 300, marginBottom: 14 }}>
            {t('Un .tar.gz con ccp.yaml (cuentas, reglas y valores por defecto) y el overlay de cada perfil. Sin secretos no lleva ni keys ni logins: se vuelven a poner al restaurar.')}
          </div>
          <Checkbox checked={withSecrets} onChange={setWithSecrets}>{t('Incluir los secretos: API keys y logins de Anthropic')}</Checkbox>
          {withSecrets && (
            <Note kind="warn" style={{ marginTop: 10 }}>{t('Con secretos, el archivo da acceso a tus cuentas: trátalo como una contraseña.')}</Note>
          )}
          <button className="btn lg primary" style={{ marginTop: 14 }} onClick={() => void exportNow()}>{t('Exportar…')}</button>
        </Card>
        <Card shadow>
          <Label style={{ marginBottom: 12 }}>{t('Restaurar')}</Label>
          <div style={{ fontSize: 12.5, color: 'var(--ink-3)', lineHeight: 1.6, fontWeight: 300, marginBottom: 14 }}>
            {t('Elige el archivo y cómo tratar lo que ya existe. Al terminar se enseña qué se creó, qué se saltó y qué se sobrescribió.')}
          </div>
          <button className="btn lg" onClick={() => void restore()}>{t('Elegir una copia…')}</button>
        </Card>
      </div>
      {report && (
        <Card style={{ marginTop: 14 }}>
          <Label style={{ marginBottom: 12 }}>{t('Resultado de la restauración')}</Label>
          <KV
            labelWidth={150}
            rows={[
              [t('Creados'), report.created.join(', ') || '—'],
              [t('Saltados'), report.skipped.join(', ') || '—'],
              [t('Sobrescritos'), report.overwritten.join(', ') || '—'],
              [t('Reglas añadidas'), String(report.rules_added)],
              [t('Foto previa'), tilde(report.snapshot) || '—'],
            ]}
          />
          <div style={{ display: 'flex', gap: 8, marginTop: 12 }}>
            <button className="btn" onClick={() => go('perfiles')}>{t('Ver las cuentas')}</button>
            {report.snapshot && <button className="btn ghost" onClick={() => void revealPath(report.snapshot)}>{t('Mostrar la foto previa')}</button>}
          </div>
        </Card>
      )}
      <CliBar cmd="ccp backup export && ccp backup restore <archivo>" />
    </div>
  );
}
