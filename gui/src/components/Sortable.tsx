// Sortable.tsx — una lista que se ordena arrastrando.
//
// Con eventos de puntero, no con el drag & drop de HTML5: el webview de Tauri
// usa ese mecanismo para soltar archivos sobre la ventana y, según la
// plataforma, se lo come antes de que llegue a la página. El puntero funciona
// igual en la app, en el navegador de desarrollo y con un trackpad.
//
// Mientras se arrastra, la fila sigue al puntero y las demás se apartan para
// dejarle hueco; al soltar, la lista se queda en el orden nuevo (optimista)
// hasta que llegan los datos releídos del motor, que son la verdad. Con el
// teclado: foco en la fila y ↑/↓ la mueven un puesto.

import { useEffect, useRef, useState, type CSSProperties, type KeyboardEvent, type PointerEvent, type ReactNode } from 'react';

interface Drag {
  from: number;
  over: number;
  dy: number;
  step: number;
}

export interface SortableRow {
  /** Props para el elemento que se arrastra (la fila entera). */
  props: {
    onPointerDown: (e: PointerEvent<HTMLElement>) => void;
    onPointerMove: (e: PointerEvent<HTMLElement>) => void;
    onPointerUp: (e: PointerEvent<HTMLElement>) => void;
    onPointerCancel: () => void;
    onKeyDown: (e: KeyboardEvent<HTMLElement>) => void;
    tabIndex: number;
    style: CSSProperties;
    'aria-roledescription': string;
  };
  /** La posición que ocupará si se suelta ahora (0 = primera). */
  position: number;
  dragging: boolean;
}

export function SortableList<T>({
  items, keyOf, onReorder, render, gap = 8, label,
}: {
  items: T[];
  keyOf: (item: T) => string;
  /** El elemento `key` pasa a la posición `to` (0 = primera). */
  onReorder: (key: string, to: number) => void;
  render: (item: T, row: SortableRow) => ReactNode;
  gap?: number;
  /** Para lectores de pantalla: qué se está ordenando. */
  label: string;
}) {
  const [drag, setDrag] = useState<Drag | null>(null);
  const [pending, setPending] = useState<string[] | null>(null);
  const refs = useRef<(HTMLDivElement | null)[]>([]);
  const origin = useRef<{ y: number; mids: number[] }>({ y: 0, mids: [] });

  // Llegaron datos nuevos del motor: mandan ellos, no el orden optimista.
  const sig = items.map(keyOf).join('\u0000');
  useEffect(() => setPending(null), [sig]);

  const byKey = new Map(items.map((it) => [keyOf(it), it]));
  const shown = pending ? pending.map((k) => byKey.get(k)).filter((x): x is T => x !== undefined) : items;

  const commit = (from: number, to: number) => {
    if (from === to || to < 0 || to >= shown.length) return;
    const keys = shown.map(keyOf);
    const [k] = keys.splice(from, 1);
    keys.splice(to, 0, k);
    setPending(keys);
    onReorder(k, to);
  };

  const offset = (j: number): number => {
    if (!drag) return 0;
    const { from, over, dy, step } = drag;
    if (j === from) return dy;
    if (from < over && j > from && j <= over) return -step;
    if (over < from && j >= over && j < from) return step;
    return 0;
  };

  const positionOf = (j: number): number => {
    if (!drag) return j;
    const { from, over } = drag;
    if (j === from) return over;
    if (from < over && j > from && j <= over) return j - 1;
    if (over < from && j >= over && j < from) return j + 1;
    return j;
  };

  return (
    <div role="list" aria-label={label}>
      {shown.map((item, j) => {
        const dragging = drag?.from === j;
        const row: SortableRow = {
          dragging,
          position: positionOf(j),
          props: {
            tabIndex: 0,
            'aria-roledescription': 'sortable',
            onPointerDown: (e) => {
              // Los botones de la fila siguen siendo botones.
              if (e.button !== 0 || (e.target as HTMLElement).closest('button, a, input, select, textarea')) return;
              const rects = refs.current.slice(0, shown.length).map((r) => r?.getBoundingClientRect());
              origin.current = { y: e.clientY, mids: rects.map((r) => (r ? r.top + r.height / 2 : 0)) };
              try {
                e.currentTarget.setPointerCapture(e.pointerId);
              } catch {
                /* un puntero que el navegador ya no reconoce: se arrastra sin captura */
              }
              setDrag({ from: j, over: j, dy: 0, step: (rects[j]?.height ?? 0) + gap });
              e.preventDefault();
            },
            onPointerMove: (e) => {
              if (!drag) return;
              const dy = e.clientY - origin.current.y;
              const center = origin.current.mids[drag.from] + dy;
              let over = 0;
              origin.current.mids.forEach((m, k) => {
                if (k !== drag.from && m < center) over++;
              });
              if (dy !== drag.dy || over !== drag.over) setDrag({ ...drag, dy, over });
            },
            onPointerUp: () => {
              if (!drag) return;
              const { from, over } = drag;
              setDrag(null);
              commit(from, over);
            },
            onPointerCancel: () => setDrag(null),
            onKeyDown: (e) => {
              if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return;
              e.preventDefault();
              commit(j, j + (e.key === 'ArrowUp' ? -1 : 1));
            },
            style: {
              transform: `translateY(${offset(j)}px)`,
              transition: dragging ? 'none' : 'transform .15s ease',
              position: 'relative',
              zIndex: dragging ? 2 : 1,
              cursor: dragging ? 'grabbing' : 'grab',
              touchAction: 'none',
              userSelect: 'none',
              boxShadow: dragging ? '0 10px 24px -12px rgba(0,0,0,.45)' : undefined,
            },
          },
        };
        return (
          <div key={keyOf(item)} role="listitem" ref={(el) => { refs.current[j] = el; }}>
            {render(item, row)}
          </div>
        );
      })}
    </div>
  );
}
