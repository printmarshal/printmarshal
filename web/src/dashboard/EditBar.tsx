import { useNavigate } from 'react-router-dom'
import { Btn } from '../components/ui'
import { Ic } from '../components/icons'

export function EditBar({ dirty, busy, isAdmin, hasPersonal, hasDefault, onDone, onCancel, onReset, onSaveDefault, onClearDefault }: {
  dirty: boolean; busy: boolean; isAdmin: boolean; hasPersonal: boolean; hasDefault: boolean
  onDone: () => void; onCancel: () => void; onReset: () => void; onSaveDefault: () => void; onClearDefault: () => void
}) {
  const navigate = useNavigate()
  return (
    <div className="edit-bar" role="region" aria-label="Customise dashboard">
      <div>
        <b>Customising your dashboard</b>
        <div className="cap">Hide, show, collapse and reorder widgets. Changes apply to your sign-in only{isAdmin ? ', unless you save them as the default for everyone' : ''}.</div>
      </div>
      <div className="row">
        {hasPersonal && <Btn sm disabled={busy} onClick={onReset}>Use the default</Btn>}
        {isAdmin && <Btn sm disabled={busy} onClick={onSaveDefault}>Save as default for everyone</Btn>}
        {isAdmin && hasDefault && <Btn sm disabled={busy} onClick={onClearDefault}>Clear default</Btn>}
        {isAdmin && <Btn sm variant="ghost" onClick={() => navigate('/settings/publicstatus')}>Public board…</Btn>}
        <Btn sm variant="ghost" disabled={busy} onClick={onCancel}>Cancel</Btn>
        <Btn sm variant="pri" disabled={busy || !dirty} onClick={onDone}><Ic.check />Done</Btn>
      </div>
    </div>
  )
}
