import { electronAPI } from '../../host/electronAPI';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ScheduleTemplate, TemplateCategory } from '../../../../../../../../third_party/cindy/scheduler/types';
import { TEMPLATE_CATEGORIES } from '../../../../../../../../third_party/cindy/scheduler/builtin-templates';

import { localizeTemplate, localizeTemplateCategory } from '../lib/templateLocalization';

interface UseTemplatesResult {
  templates: ScheduleTemplate[];
  categories: TemplateCategory[];
  loading: boolean;
  error: string | null;
}

export function useTemplates(): UseTemplatesResult {
  // useTranslation 订阅语言切换，语言变化时本地化结果跟着重算。
  const { i18n } = useTranslation();
  const [rawTemplates, setRawTemplates] = useState<ScheduleTemplate[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const list = (await electronAPI.maker.schedule.listTemplates()) as ScheduleTemplate[];
        if (!cancelled) setRawTemplates(list);
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const language = i18n.language;
  const templates = useMemo(
    () => rawTemplates.map((template) => localizeTemplate(i18n, template)),
    [rawTemplates, i18n, language],
  );
  const categories = useMemo(
    () => TEMPLATE_CATEGORIES.map((category) => localizeTemplateCategory(i18n, category)),
    [i18n, language],
  );

  return { templates, categories, loading, error };
}
