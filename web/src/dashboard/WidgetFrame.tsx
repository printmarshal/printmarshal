import type { ReactNode } from 'react'
import { Btn } from '../components/ui'
import { Ic } from '../components/icons'
import type { WidgetDef } from './catalogue'

/**
 * Wraps a widget on the grid: its column span, its collapsed state, and in
 * edit mode a toolbar with real buttons for moving, hiding and collapsing.
 */
export function WidgetFrame({ def, editing, collapsed, first, last, onMove, onHide, onCollapse, children }: {
  def: WidgetDef; editing: boolean; collapsed: boolean; first: boolean; last: boolean
  onMove: (dir: -1 | 1) => void; onHide: () => void; onCollapse: (v: boolean) => void; children: ReactNode
}) {
  const canCollapse = def.size !== 'kpi'
  return (
    <div className={`widget span-${def.size}${collapsed ? ' collapsed' : ''}${editing ? ' editing' : ''}`} data-widget={def.id}>
      {editing && (
        <div className="widget-tools" role="group" aria-label={`${def.title} widget controls`}>
          <span className="widget-title">{def.title}</span>
          <Btn sm ic aria-label={`Move ${def.title} up`} disabled={first} onClick={() => onMove(-1)}><Ic.arrowUp /></Btn>
          <Btn sm ic aria-label={`Move ${def.title} down`} disabled={last} onClick={() => onMove(1)}><Ic.chevronDown /></Btn>
          {canCollapse && <Btn sm onClick={() => onCollapse(!collapsed)}>{collapsed ? 'Expand' : 'Collapse'}</Btn>}
          <Btn sm onClick={onHide}>Hide</Btn>
        </div>
      )}
      {collapsed && canCollapse ? (
        <section className="card widget-collapsed" aria-label={def.title}><div className="ch"><h2 className="h2">{def.title}</h2><span className="cap">collapsed</span></div></section>
      ) : children}
    </div>
  )
}
