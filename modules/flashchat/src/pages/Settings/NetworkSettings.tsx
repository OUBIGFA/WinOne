import { createContext, MutableRefObject, useContext, useEffect, useState } from 'react'
import { Alert, Box, Button, TextField, Accordion, AccordionSummary, AccordionDetails, Typography } from '@mui/material'
import ExpandMore from '@mui/icons-material/ExpandMore'
import { SettingsContext } from 'contexts/SettingsContext'
import { validNetworkSettings } from 'core/config/networkSettings'
import { t, useLanguage } from 'config/i18n'

interface NetworkDraft { rtc: string; trackers: string; baseline: string }
export const NetworkDraftContext = createContext<MutableRefObject<NetworkDraft | null>>({ current: null })

/** Keep the original WinOne custom network controls alongside upstream's
 * complete settings. Overrides apply to both chat and encrypted file transfer. */
export function NetworkSettings() {
  useLanguage()
  const { getUserSettings, updateUserSettings } = useContext(SettingsContext)
  const settings = getUserSettings()
  const draft = useContext(NetworkDraftContext)
  const savedRtc = settings.rtcConfig ? JSON.stringify(settings.rtcConfig, null, 2) : ''
  const savedTrackers = settings.trackerUrls?.join('\n') ?? ''
  const baseline = JSON.stringify([savedRtc, savedTrackers])
  const initial = draft.current?.baseline === baseline ? draft.current : { rtc: savedRtc, trackers: savedTrackers }
  const [rtc, setRtc] = useState(initial.rtc)
  const [trackers, setTrackers] = useState(initial.trackers)
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)
  const [saving, setSaving] = useState(false)
  // Flash Chat allows free navigation. Keep the unapplied form in this window,
  // and reset it when profile import or another save replaces its baseline.
  useEffect(() => {
    if (draft.current && draft.current.baseline !== baseline) {
      setRtc(savedRtc); setTrackers(savedTrackers); draft.current = null
    }
  }, [baseline, savedRtc, savedTrackers, draft])
  const edit = (nextRtc: string, nextTrackers: string) => {
    setRtc(nextRtc); setTrackers(nextTrackers); setSaved(false)
    draft.current = { rtc: nextRtc, trackers: nextTrackers, baseline }
  }
  const save = async () => {
    setSaving(true)
    try {
      const overrides = { rtcConfig: rtc.trim() ? JSON.parse(rtc) : undefined, trackerUrls: trackers.split('\n').map(s => s.trim()).filter(Boolean) }
      if (!validNetworkSettings(overrides)) throw new Error(t('Invalid network configuration'))
      await updateUserSettings(overrides)
      setError(''); setSaved(true)
    } catch (error) { setError(error instanceof Error ? error.message : String(error)); setSaved(false) }
    finally { setSaving(false) }
  }
  return <Accordion elevation={0} sx={{ my: 2, border: '1px solid', borderColor: 'divider' }}>
    <AccordionSummary expandIcon={<ExpandMore />}><Typography>{t('Custom network settings')}</Typography></AccordionSummary>
    <AccordionDetails><Box sx={{ display: 'grid', gap: 2 }}>
      <Typography variant="body2">{t('Leave blank to use the public defaults. Changes apply when joining a room.')}</Typography>
      <TextField disabled={saving} label={t('Pairing servers (one WebSocket URL per line)')} multiline minRows={2} value={trackers} onChange={e => edit(rtc, e.target.value)} />
      <TextField disabled={saving} label={t('WebRTC configuration (JSON)')} multiline minRows={3} value={rtc} onChange={e => edit(e.target.value, trackers)} />
      {error && <Alert severity="error" onClose={() => setError('')}>{error}</Alert>}
      {saved && <Alert severity="success" onClose={() => setSaved(false)}>{t('Network settings saved')}</Alert>}
      <Button disabled={saving} variant="outlined" onClick={save}>{t('Apply network settings')}</Button>
    </Box></AccordionDetails>
  </Accordion>
}
