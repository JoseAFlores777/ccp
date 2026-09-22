// Glosario — el diccionario entero en una pantalla. Los «?» repartidos por la
// app explican un término donde aparece; aquí están todos juntos, por tema, y
// se buscan. El texto es el mismo (lib/glossary.ts): no hay dos versiones.

import { useMemo, useState } from 'react';
import { GLOSSARY_GROUPS, glossary, glossaryGroup, glossaryKeys, type GlossaryGroup } from '../lib/glossary';
import { t } from '../lib/i18n';
import { Card, Empty } from '../components/ui';

function groupName(g: GlossaryGroup): string {
  return {
    cuentas: t('Cuentas y carpetas'),
    conversaciones: t('Conversaciones'),
    rotacion: t('Rotación'),
    configuracion: t('Configuración'),
    desktop: t('Desktop'),
    historial: t('Historial y nube'),
    app: t('La app'),
  }[g];
}

export function Glosario() {
  const [q, setQ] = useState('');
  const groups = useMemo(() => {
    const s = q.trim().toLowerCase();
    return GLOSSARY_GROUPS.map((g) => ({
      g,
      keys: glossaryKeys().filter((k) => {
        if (glossaryGroup(k) !== g) return false;
        if (!s) return true;
        const e = glossary(k)!;
        return [e.term, e.what, e.why, e.how].join(' ').toLowerCase().includes(s);
      }),
    })).filter((x) => x.keys.length > 0);
  }, [q]);

  return (
    <div>
      <input
        className="input"
        style={{ fontFamily: 'inherit', width: '100%', marginBottom: 16 }}
        value={q}
        onChange={(e) => setQ(e.target.value)}
        placeholder={t('Buscar un término: préstamo, cadena, MCP…')}
        autoFocus
      />
      {groups.length === 0 && (
        <Card>
          <Empty title={t('Nada coincide con «{q}»', { q })} />
        </Card>
      )}
      {groups.map(({ g, keys }) => (
        <section key={g} style={{ marginBottom: 22 }}>
          <div className="label" style={{ margin: '0 0 10px 2px' }}>{groupName(g)}</div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))', gap: 12 }}>
            {keys.map((k) => {
              const e = glossary(k)!;
              return (
                <Card key={k} style={{ padding: '14px 16px' }}>
                  <div style={{ fontSize: 13.5, color: 'var(--ink)', fontWeight: 500, marginBottom: 8 }}>{e.term}</div>
                  {([[t('Qué es'), e.what], [t('Para qué'), e.why], [t('Cómo funciona'), e.how]] as const).map(([label, text]) => (
                    <div key={label} style={{ fontSize: 12, color: 'var(--ink-2)', lineHeight: 1.6, fontWeight: 300, marginBottom: 7 }}>
                      <span className="label" style={{ display: 'block', fontSize: 9 }}>{label}</span>
                      {text}
                    </div>
                  ))}
                </Card>
              );
            })}
          </div>
        </section>
      ))}
    </div>
  );
}
