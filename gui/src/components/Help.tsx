// Help.tsx — el «?» del diccionario: pasar por encima explica el término.
//
// El tooltip se pinta en un portal sobre <body>, con posición fija: muchas
// tarjetas llevan overflow:hidden y un tooltip dentro de ellas saldría cortado.
// Se abre con hover y con foco (teclado), y se queda abierto mientras el puntero
// está encima de él, para poder llegar al enlace «Ver en el glosario».

import { useEffect, useLayoutEffect, useRef, useState, type CSSProperties } from 'react';
import { createPortal } from 'react-dom';
import { glossary } from '../lib/glossary';
import { t } from '../lib/i18n';
import { useApp } from '../lib/store';

const W = 320;

export function Help({ term, size = 14, style }: { term: string; size?: number; style?: CSSProperties }) {
  const g = glossary(term);
  const { go } = useApp();
  const anchor = useRef<HTMLSpanElement>(null);
  const tip = useRef<HTMLDivElement>(null);
  const closeTimer = useRef<number | undefined>(undefined);
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<{ left: number; top: number; above: boolean } | null>(null);

  const show = () => {
    window.clearTimeout(closeTimer.current);
    setOpen(true);
  };
  // Un respiro antes de cerrar: da tiempo a cruzar del icono al tooltip.
  const hide = () => {
    window.clearTimeout(closeTimer.current);
    closeTimer.current = window.setTimeout(() => setOpen(false), 140);
  };
  useEffect(() => () => window.clearTimeout(closeTimer.current), []);

  useLayoutEffect(() => {
    if (!open || !anchor.current) return;
    const r = anchor.current.getBoundingClientRect();
    const h = tip.current?.offsetHeight ?? 180;
    const left = Math.min(Math.max(8, r.left + r.width / 2 - W / 2), window.innerWidth - W - 8);
    const below = r.bottom + 8;
    const above = below + h > window.innerHeight - 8 && r.top - h - 8 > 8;
    setPos({ left, top: above ? r.top - h - 8 : below, above });
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false);
    const onScroll = () => setOpen(false);
    window.addEventListener('keydown', onKey);
    window.addEventListener('scroll', onScroll, true);
    return () => {
      window.removeEventListener('keydown', onKey);
      window.removeEventListener('scroll', onScroll, true);
    };
  }, [open]);

  if (!g) return null;

  return (
    <>
      <span
        ref={anchor}
        role="button"
        tabIndex={0}
        aria-label={t('Qué es «{t}»', { t: g.term })}
        aria-expanded={open}
        className="help-dot"
        onMouseEnter={show}
        onMouseLeave={hide}
        onFocus={show}
        onBlur={hide}
        // Dentro de una pestaña o una fila con clic propio, el «?» no debe
        // además cambiar de pestaña: se queda en explicar.
        onClick={(e) => {
          e.stopPropagation();
          e.preventDefault();
          setOpen((o) => !o);
        }}
        onPointerDown={(e) => e.stopPropagation()}
        style={{ width: size, height: size, fontSize: Math.round(size * 0.7), ...style }}
      >
        ?
      </span>
      {open &&
        createPortal(
          <div
            ref={tip}
            role="tooltip"
            className="help-tip fade-in"
            onMouseEnter={show}
            onMouseLeave={hide}
            style={{ left: pos?.left ?? -9999, top: pos?.top ?? -9999, width: W }}
          >
            <div className="help-term">{g.term}</div>
            <div className="help-sec"><span>{t('Qué es')}</span>{g.what}</div>
            <div className="help-sec"><span>{t('Para qué')}</span>{g.why}</div>
            <div className="help-sec"><span>{t('Cómo funciona')}</span>{g.how}</div>
            <button
              className="help-more"
              onClick={() => {
                setOpen(false);
                go('glosario');
              }}
            >
              {t('Ver todo el glosario')} →
            </button>
          </div>,
          document.body,
        )}
    </>
  );
}
