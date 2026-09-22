// Boundary — la red que faltaba: una excepción al pintar deja de tirar la app.
//
// React desmonta el árbol ENTERO cuando un render lanza y no hay boundary. En
// una pestaña eso es una página en blanco y F5; en una app de escritorio es una
// ventana negra sin menú útil y matar el proceso, que es lo que pasaba al editar
// un servidor MCP. El error real quedaba en una consola que nadie abre.
//
// Lo que esta clase compra no es evitar el fallo —eso se arregla en su sitio—
// sino que el fallo se pueda CONTAR: qué pasó, dónde, y un botón para seguir.
// Sin esto, cualquier descuido futuro en una vista vuelve a costar la sesión
// entera del usuario.
//
// Es una clase porque componentDidCatch/getDerivedStateFromError no tienen
// equivalente en hooks. No es preferencia de estilo: React no ofrece otra cosa.

import { Component, type ErrorInfo, type ReactNode } from 'react';

type Props = { children: ReactNode };
type State = { error: Error | null; stack: string };

export class Boundary extends Component<Props, State> {
  state: State = { error: null, stack: '' };

  static getDerivedStateFromError(error: Error): Partial<State> {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // La pila de COMPONENTES es lo que dice en qué vista pasó; el stack de JS a
    // secas apunta a react-dom y no ubica nada. Se guarda para poder copiarla:
    // un error que no se puede pegar en un issue obliga a reproducirlo a ciegas.
    this.setState({ stack: (info.componentStack ?? '').trim() });
    console.error('[ccp] render fallido', error, info.componentStack);
  }

  render() {
    const { error, stack } = this.state;
    if (!error) return this.props.children;

    const detail = [error.message, error.stack ?? '', stack].filter(Boolean).join('\n\n');
    return (
      <div style={{ padding: 28, maxWidth: 760, margin: '0 auto', color: 'var(--ink)' }}>
        <h2 style={{ fontSize: 17, margin: '0 0 6px' }}>Algo se rompió al pintar esta pantalla</h2>
        <p style={{ fontSize: 13, color: 'var(--ink-3)', fontWeight: 300, lineHeight: 1.5, margin: '0 0 16px' }}>
          Es un fallo de la interfaz, no de tu configuración: nada se ha escrito. Puedes volver y seguir
          usando el resto.
        </p>
        <div style={{ display: 'flex', gap: 8, marginBottom: 18 }}>
          <button className="btn primary" onClick={() => this.setState({ error: null, stack: '' })}>
            Volver
          </button>
          <button className="btn" onClick={() => window.location.reload()}>
            Recargar la app
          </button>
          <button className="btn" onClick={() => void navigator.clipboard?.writeText(detail)}>
            Copiar el detalle
          </button>
        </div>
        <pre
          className="mono selectable"
          style={{
            fontSize: 11, lineHeight: 1.5, whiteSpace: 'pre-wrap', color: 'var(--ink-3)',
            background: 'var(--surface-2)', border: '1px solid var(--line)', borderRadius: 8,
            padding: 12, maxHeight: 320, overflow: 'auto', margin: 0,
          }}
        >
          {detail}
        </pre>
      </div>
    );
  }
}
