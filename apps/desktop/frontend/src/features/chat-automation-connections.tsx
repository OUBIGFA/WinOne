// Connector method mappings for automation script capabilities and the
// Feishu / WeCom completion channels. They reuse the MCP servers configured
// here; Cindy's automation page reads their readiness (capabilityStatus,
// channels). Credentials stay in the MCP server settings.
import {useEffect, useState} from 'react';
import {chatCall} from '../bridge';
import {i18n, useTranslation} from '../i18n';
import {automationMessages} from '../locales/automations';
import {automationCompleteMessages} from '../locales/automations-complete';
import {Button} from '../ui/button';
import {refreshAutomationChannels} from './cindy/host/notifications';

for (const [locale, messages] of Object.entries(automationMessages)) i18n.addResourceBundle(locale, 'automations', messages, true, true);
for (const [locale, messages] of Object.entries(automationCompleteMessages)) i18n.addResourceBundle(locale, 'automations', messages, true, true);

export function AutomationConnections() {
  const {t} = useTranslation('automations');
  const [json, setJSON] = useState(''), [saved, setSaved] = useState(''), [servers, setServers] = useState<string[]>([]);
  const [busy, setBusy] = useState(true), [error, setError] = useState(''), [notice, setNotice] = useState('');
  const load = async () => {
    setBusy(true); setError('');
    try {
      const value = await chatCall<{settings: unknown; servers: Array<{name: string; enabled: boolean}>}>('automations.connections.get');
      const text = JSON.stringify(value.settings, null, 2);
      setJSON(text); setSaved(text); setServers(value.servers.filter((server) => server.enabled).map((server) => server.name));
    } catch (failure) { setError(failure instanceof Error ? failure.message : String(failure)); } finally { setBusy(false); }
  };
  useEffect(() => { void load(); }, []);
  const save = async () => {
    setBusy(true); setError(''); setNotice('');
    try {
      await chatCall('automations.connections.save', {settings: JSON.parse(json)});
      setSaved(json); setNotice(t('save'));
      await refreshAutomationChannels();
    } catch (failure) { setError(failure instanceof Error ? failure.message : String(failure)); } finally { setBusy(false); }
  };
  return <section className="chat-settings-section" aria-label={t('connections')}>
    <h3>{t('connections')}</h3>
    <p className="chat-settings-hint">{t('connectionsHint')}</p>
    <p className="chat-settings-hint">{t('configuredServers')} · {servers.join(' · ') || t('notConfigured')}</p>
    <label className="chat-dialog-field">{t('connectionsJSON')}<textarea rows={10} spellCheck={false} value={json} disabled={busy} onChange={(event) => { setJSON(event.target.value); setNotice(''); }}/></label>
    <p className="chat-settings-hint">jira.get · jira.search_jql · jira.add_comment · feishu.recent_chats · feishu.recent_messages · notify.feishu · notify.wecom</p>
    {error && <p role="alert" className="chat-settings-feedback">{error}</p>}
    {notice && !error && <p role="status" className="chat-settings-hint">{notice}</p>}
    <div className="chat-settings-row"><Button disabled={busy || json === saved} onClick={() => void save()}>{t('save')}</Button></div>
  </section>;
}
