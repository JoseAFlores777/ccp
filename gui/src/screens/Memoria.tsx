// P-15 Memoria de Claude — las instrucciones y artefactos que ccp gestiona,
// por alcance: global (todas las cuentas), perfil (una cuenta) y proyecto (el
// repo de la carpeta en contexto). Solo se ve lo que ccp creó.

import { useState } from 'react';
import { api, type MemoryItem } from '../lib/api';
import { shellJoin, tilde } from '../lib/format';
import { buildMcp } from '../lib/mcp';
import { t } from '../lib/i18n';
import { useApp, useCall, type ModalSpec } from '../lib/store';
import { Card, CliBar, Empty, ErrorNote, Loading, Note, Segmented } from '../components/ui';

type Scope = 'global' | 'profile' | 'project';

const TYPES: Record<Scope, string[]> = {
  global: ['rule', 'command', 'agent', 'skill', 'hook', 'mcp'],
  profile: ['rule', 'hook'],
  project: ['rule', 'command', 'agent', 'skill', 'hook', 'mcp'],
};

function typeName(ty: string): string {
  switch (ty) {
    case 'rule': return t('Regla');
    case 'command': return t('Comando');
    case 'agent': return t('Agente');
    case 'skill': return t('Skill');
    case 'hook': return t('Hook');
    case 'mcp': return 'MCP';
  }
  return ty;
}

function scopeName(s: Scope): string {
  return s === 'global' ? t('Global') : s === 'profile' ? t('Perfil') : t('Proyecto');
}

function addModal(scope: Scope, ctx: { profile?: string; cwd?: string; repo?: string }): ModalSpec {
  const isMcp = (f: Record<string, string>) => f.type === 'mcp';
  const withName = (f: Record<string, string>) => f.type !== 'rule';
  const mcpDest = scope === 'global' ? '~/.claude.json' : `${tilde(ctx.repo) || '<repo>'}/.mcp.json`;
  const transport = (want: string[]) => (f: Record<string, string>) => isMcp(f) && want.includes(f.transport || 'stdio');
  return {
    title: t('Añadir a {s}', { s: scopeName(scope) }),
    sub: t('Lo mismo se puede pedir en lenguaje natural dentro de Claude Code con /ccp:remember-{s}.', { s: scope === 'profile' ? 'profile' : scope }),
    initial: { type: 'rule', name: '', text: '', transport: 'stdio', command: '', args: '', env: '', url: '', headers: '', json: '' },
    fields: [
      { key: 'type', label: t('Tipo'), kind: 'select', options: TYPES[scope].map((v) => ({ value: v, label: typeName(v) })) },
      {
        key: 'name', label: t('Nombre'), kind: 'text', show: withName,
        hint: t('Para un MCP, el nombre con el que lo verá Claude Code; para un hook, su clave; para comandos, agentes y skills, el nombre del archivo.'),
      },
      {
        key: 'transport', label: t('Cómo se conecta'), kind: 'select', show: isMcp,
        options: [
          { value: 'stdio', label: t('Programa local (stdio): Claude Code lo arranca') },
          { value: 'http', label: t('Servidor remoto por HTTP (una URL)') },
          { value: 'sse', label: t('Servidor remoto por SSE (una URL, la forma antigua)') },
          { value: 'json', label: t('Avanzado: pegar la entrada en JSON') },
        ],
      },
      {
        key: 'command', label: t('Programa'), kind: 'text', show: transport(['stdio']), placeholder: 'npx',
        hint: t('Solo el ejecutable: npx, uvx, docker, node o una ruta absoluta.'),
      },
      {
        key: 'args', label: t('Argumentos'), kind: 'area', show: transport(['stdio']),
        placeholder: '-y\n@modelcontextprotocol/server-filesystem\n/ruta/al/proyecto',
        hint: t('Uno por línea, sin comillas: cada línea es un argumento aunque lleve espacios.'),
      },
      {
        key: 'env', label: t('Variables de entorno'), kind: 'area', show: transport(['stdio']),
        placeholder: 'API_KEY=${API_KEY}\nLOG_LEVEL=info',
        hint: t('NOMBRE=valor, una por línea. ${VARIABLE} toma el valor de tu entorno al arrancar, así el secreto no se guarda en el archivo.'),
      },
      {
        key: 'url', label: t('URL'), kind: 'text', show: transport(['http', 'sse']), placeholder: 'https://mcp.ejemplo.com/mcp',
      },
      {
        key: 'headers', label: t('Cabeceras'), kind: 'area', show: transport(['http', 'sse']),
        placeholder: 'Authorization: Bearer ${TOKEN}',
        hint: t('Nombre: valor, una por línea. ${VARIABLE} toma el valor de tu entorno, así el token no se guarda en el archivo.'),
      },
      {
        key: 'json', label: t('Entrada del servidor (JSON)'), kind: 'area', show: transport(['json']),
        placeholder: '{"command": "npx", "args": ["-y", "paquete"], "env": {"CLAVE": "${CLAVE}"}}',
        hint: t('Lo que iría dentro de mcpServers para este nombre, sin el nombre.'),
      },
      {
        key: 'text', label: t('Contenido'), kind: 'area', show: (f) => !isMcp(f),
        hint: t('Una regla es una línea de instrucciones. Un hook es su JSON. Un comando, agente o skill es markdown: la primera línea es su descripción.'),
      },
    ],
    preview: (f) => {
      if (!isMcp(f)) return null;
      const b = buildMcp(f);
      if (!b.config) return null;
      return { label: t('Se añadirá a {d}', { d: mcpDest }), text: JSON.stringify({ mcpServers: { [f.name.trim()]: b.config } }, null, 2) };
    },
    warns: (f) => {
      if (isMcp(f)) {
        const b = buildMcp(f);
        const touched = (f.name ?? '') + (f.command ?? '') + (f.url ?? '') + (f.json ?? '') !== '';
        const w = touched ? [...b.errors, ...b.warnings] : [];
        if (scope === 'project') {
          w.push(t('Va a {d}, que suele subirse a git: lo verá cualquiera con acceso al repo. Claude Code pedirá aprobarlo la primera vez.', { d: mcpDest }));
        } else {
          w.push(t('Va a {d}, la configuración de default: las demás cuentas tienen su propio .claude.json y no lo verán. Para tenerlo en un repo con cualquier cuenta, añádelo en Proyecto.', { d: mcpDest }));
        }
        w.push(t('Las sesiones ya abiertas no lo ven hasta reiniciarlas; /mcp dentro de Claude Code dice si conectó.'));
        return w;
      }
      if (f.type !== 'hook' || !(f.text ?? '').trim()) return [];
      try {
        JSON.parse(f.text);
        return [];
      } catch {
        return [t('El contenido no es JSON válido todavía.')];
      }
    },
    canConfirm: (f) => {
      if (isMcp(f)) return buildMcp(f).config !== undefined;
      if (!(f.text ?? '').trim()) return false;
      if (withName(f) && !(f.name ?? '').trim()) return false;
      if (f.type === 'hook') {
        try {
          JSON.parse(f.text);
        } catch {
          return false;
        }
      }
      return true;
    },
    confirmLabel: t('Añadir'),
    cli: (f) => {
      const pre = scope === 'profile' && ctx.profile ? `ccp use ${ctx.profile} && ` : '';
      if (isMcp(f)) {
        const b = buildMcp(f);
        const text = `${(f.name ?? '').trim() || '<nombre>'}=${b.config ? JSON.stringify(b.config) : '{…}'}`;
        return pre + shellJoin(['ccp', 'instruct', 'add', scope, 'mcp', text]);
      }
      const text = f.type === 'rule' ? '…' : `${f.name || '<nombre>'}=…`;
      return pre + shellJoin(['ccp', 'instruct', 'add', scope, f.type ?? 'rule', text]);
    },
    onConfirm: async (f) => {
      if (isMcp(f)) {
        const b = buildMcp(f);
        if (!b.config) throw new Error(b.errors.join(' '));
        const name = f.name.trim();
        const r = await api.addMemory({ scope, cwd: ctx.cwd, type: 'mcp', name, text: JSON.stringify(b.config) });
        return t('MCP {n} añadido en {d}', { n: name, d: tilde(r.dest) });
      }
      const r = await api.addMemory({ scope, profile: ctx.profile, cwd: ctx.cwd, type: f.type, text: f.text.trim(), name: f.name?.trim() });
      if (r.duplicate) return t('Ya estaba: no se ha duplicado');
      return t('Añadido en {d}', { d: tilde(r.dest) });
    },
  };
}

function removeModal(scope: Scope, it: MemoryItem, ctx: { profile?: string; cwd?: string }): ModalSpec {
  const warn = (() => {
    switch (it.type) {
      case 'rule': return t('Se quita la línea del bloque que gestiona ccp. Lo escrito a mano en ese archivo no se toca.');
      case 'hook': return t('ccp deja de recordarlo, pero el hook sigue en su settings.json: no tiene un id estable con el que borrarlo sin riesgo. Quítalo a mano si ya no lo quieres.');
      case 'mcp': return t('Se borra su entrada del archivo de MCP.');
      case 'skill': return t('Se borra la carpeta de la skill.');
      default: return t('Se borra su archivo.');
    }
  })();
  return {
    title: t('Olvidar este artefacto'),
    sub: it.text,
    warns: [warn],
    danger: it.type !== 'rule' && it.type !== 'hook',
    confirmLabel: t('Olvidar'),
    cli: () => (scope === 'profile' && ctx.profile ? `ccp use ${ctx.profile} && ` : '') + shellJoin(['ccp', 'instruct', 'rm', scope, String(it.index)]),
    onConfirm: async () => {
      await api.removeMemory({ scope, profile: ctx.profile, cwd: ctx.cwd, index: it.index });
      return t('Olvidado');
    },
  };
}

export function Memoria() {
  const app = useApp();
  const { profiles, selected, folder, openModal } = app;
  const [scope, setScope] = useState<Scope>('global');
  const official = profiles.filter((p) => p.name !== 'default');
  const [profile, setProfile] = useState<string>(selected !== 'default' ? selected : official[0]?.name ?? '');
  const ctx = { profile: scope === 'profile' ? profile : undefined, cwd: scope === 'project' ? folder : undefined };
  const res = useCall(() => api.memory({ scope, ...ctx }), [scope, profile, folder]);

  return (
    <div>
      <div style={{ display: 'flex', gap: 12, alignItems: 'center', marginBottom: 16, flexWrap: 'wrap' }}>
        <Segmented<Scope>
          value={scope}
          onChange={setScope}
          options={[
            { value: 'global', label: t('Global') },
            { value: 'profile', label: t('Perfil') },
            { value: 'project', label: t('Proyecto') },
          ]}
        />
        <span style={{ flex: 1 }} />
        {scope === 'profile' && (
          <select className="input" style={{ width: 200, padding: '6px 10px' }} value={profile} onChange={(e) => setProfile(e.target.value)}>
            {official.map((p) => (
              <option key={p.name} value={p.name}>{p.name}</option>
            ))}
          </select>
        )}
        {scope === 'project' && res.data?.repo && <span className="mono ellipsis" style={{ fontSize: 11, color: 'var(--ink-4)', maxWidth: 360 }}>{tilde(res.data.repo)}</span>}
      </div>

      {res.error && <ErrorNote error={res.error} onRetry={res.reload} />}
      {!res.data && !res.error && <Loading rows={4} />}
      {res.data && !res.data.available && (
        <Card>
          <Empty title={res.data.reason === 'no_repo' ? t('La carpeta en contexto no es un repo git') : t('default no tiene memoria de perfil')}>
            {res.data.reason === 'no_repo'
              ? t('La memoria de proyecto vive en el .claude/ del repo. Elige arriba una carpeta que esté dentro de un repo git.')
              : t('default es tu ~/.claude tal cual: lo que quieras que lea va en Global.')}
          </Empty>
        </Card>
      )}
      {res.data?.available && (
        <Card pad={false} clip shadow>
          {res.data.items.length === 0 && (
            <Empty title={t('Nada todavía')}>{t('Aquí aparecerá lo que ccp añada a la memoria de Claude en este alcance.')}</Empty>
          )}
          {res.data.items.map((m) => (
            <div key={m.index} style={{ display: 'flex', gap: 14, alignItems: 'flex-start', padding: '14px 18px', borderBottom: '1px solid var(--line-soft)' }}>
              <span className="mono" style={{ fontSize: 9.5, letterSpacing: '.08em', textTransform: 'uppercase', color: 'var(--ink-4)', width: 62, flex: '0 0 62px', paddingTop: 3 }}>
                {typeName(m.type)}
              </span>
              <span style={{ flex: 1, minWidth: 0 }}>
                <span className="selectable" style={{ display: 'block', fontSize: 12.5, color: 'var(--ink)', lineHeight: 1.55 }}>{m.text}</span>
                <span className="mono ellipsis" style={{ display: 'block', fontSize: 10.5, color: 'var(--ink-4)', marginTop: 4 }}>{tilde(m.where)}</span>
              </span>
              <button className="btn quiet danger sm" onClick={() => openModal(removeModal(scope, m, ctx))}>
                {m.type === 'hook' ? t('Olvidar') : t('Borrar')}
              </button>
            </div>
          ))}
          <div style={{ padding: '12px 18px' }}>
            <button className="btn dashed" onClick={() => openModal(addModal(scope, { ...ctx, repo: res.data?.repo }))}>
              {t('Añadir a {s}', { s: scopeName(scope) })}
            </button>
          </div>
        </Card>
      )}
      <Note style={{ marginTop: 14 }}>
        {t('Aquí solo se ve lo que ccp creó. Lo que hayas escrito a mano en esos archivos no aparece y no se toca nunca. Lo mismo se puede pedir en lenguaje natural con /ccp:remember-global.')}
      </Note>
      <CliBar cmd={`ccp instruct list ${scope}`} />
    </div>
  );
}
