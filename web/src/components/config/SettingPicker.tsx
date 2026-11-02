import { useId, useMemo, useRef, useState } from 'react'
import { Ic } from '../icons'
import { Tag } from '../ui'
import type { CatalogDef } from '../../types-config'
import { areaLabel } from '../../types-config'

const typeWords: Record<string, string> = { bool: 'on or off', int: 'number', enum: 'choice', list: 'list', string: 'text' }

/** The data a dragged row carries, so a drop can only add a setting from this picker. */
export const settingDragType = 'application/x-printmarshal-setting'

/**
 * Finds a setting by what it does: search names, keys, explanations and areas,
 * then click, press Enter or drag a row into the profile. The list keeps its
 * place as settings are added, so several can be added in a row.
 */
export default function SettingPicker({ settings, areas, onAdd, onDragging }: { settings: CatalogDef[]; areas: string[]; onAdd: (key: string) => void; onDragging: (dragging: boolean) => void }) {
  const [query, setQuery] = useState('')
  const [area, setArea] = useState('')
  const searchRef = useRef<HTMLInputElement>(null)
  const listId = useId()
  const counts = useMemo(() => new Map(areas.map((a) => [a, settings.filter((d) => d.area === a).length])), [areas, settings])
  const words = query.trim().toLowerCase().split(/\s+/).filter(Boolean)
  const shown = settings.filter((d) => {
    if (area && d.area !== area) return false
    const text = `${d.name} ${d.key} ${d.help || ''} ${areaLabel(d.area)}`.toLowerCase()
    return words.every((w) => text.includes(w))
  })
  const add = (key: string) => { onAdd(key); searchRef.current?.focus() }
  return (
    <div className="sp">
      <div className="search sm">
        <Ic.search />
        <input ref={searchRef} type="search" aria-label="Search settings" aria-controls={listId} placeholder="Search by what it does, for example sleep or telnet" value={query} onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); if (shown[0]) add(shown[0].key) } }} />
      </div>
      <div className="sp-areas" role="group" aria-label="Filter by area">
        <button type="button" className={`chip pillish${area === '' ? ' set' : ''}`} aria-pressed={area === ''} onClick={() => setArea('')}>All <span className="num">{settings.length}</span></button>
        {areas.filter((a) => counts.get(a)).map((a) => <button type="button" key={a} className={`chip pillish${area === a ? ' set' : ''}`} aria-pressed={area === a} onClick={() => setArea(area === a ? '' : a)}>{areaLabel(a)} <span className="num">{counts.get(a)}</span></button>)}
      </div>
      <div className="cap" role="status">{shown.length === 0 ? `No setting matches “${query.trim()}”.` : `${shown.length} setting${shown.length === 1 ? '' : 's'}. Click to add, or drag into the profile.${words.length ? ' Enter adds the first.' : ''}`}</div>
      <ul className="sp-list" id={listId}>
        {shown.map((d) => (
          <li key={d.key} draggable onDragStart={(e) => { e.dataTransfer.setData(settingDragType, d.key); e.dataTransfer.effectAllowed = 'copy'; onDragging(true) }} onDragEnd={() => onDragging(false)}>
            <button type="button" className="sp-row" aria-label={`Add ${d.name}`} onClick={() => add(d.key)}>
              <span className="sp-text">
                <span className="sp-name">{d.name}</span>
                {d.help && <span className="cap">{d.help}</span>}
                <span className="sp-meta"><span className="mono">{d.key}</span><Tag>{typeWords[d.type] || d.type}</Tag></span>
              </span>
              <Ic.plus aria-hidden="true" />
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}
