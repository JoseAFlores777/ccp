// Markdown.tsx — el texto de una conversación, legible.
//
// react-markdown con GFM (tablas, listas de tareas, tachado). Tres decisiones:
//   - HTML incrustado no se interpreta (es lo que hace react-markdown por
//     defecto y no se cambia): un transcript viene de fuera y no puede meter
//     nada ejecutable en la app.
//   - Un enlace no navega: la ventana de la app ES la app, y seguir un enlace la
//     sacaría de sí misma. Pulsarlo copia la URL y lo dice.
//   - Las imágenes no se cargan (la CSP solo deja las locales): se enseña su
//     texto alternativo.

import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { copyText } from '../lib/bridge';
import { t } from '../lib/i18n';
import { useApp } from '../lib/store';

export function Markdown({ text, keepBreaks = false }: { text: string; keepBreaks?: boolean }) {
  const { notify } = useApp();
  return (
    <div className={`md selectable${keepBreaks ? ' keep-breaks' : ''}`}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          a: ({ href, children }) => (
            <a
              href={href}
              title={t('Copiar el enlace')}
              onClick={async (e) => {
                e.preventDefault();
                if (href && (await copyText(href))) notify(t('Enlace copiado: {u}', { u: href }), 'info');
              }}
            >
              {children}
            </a>
          ),
          img: ({ alt }) => <span className="md-img">[{alt || t('imagen')}]</span>,
        }}
      >
        {text}
      </ReactMarkdown>
    </div>
  );
}
