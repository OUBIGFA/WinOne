// WinOne copy around Cindy's teammate pages (the pages' own text comes from
// the generated Cindy locale subset in features/cindy/locales).
export const onebotMessages = {
 zh: {
  title: 'OneBot',
  loading: '正在打开 OneBot',
  loadFailed: 'OneBot 暂时无法打开',
  retry: '重试',
  teammates: '伙伴',
  connectAI: {
   title: '先连接一个 AI 服务',
   description: '伙伴使用 AI 服务中已启用的模型。连接服务并启用模型后，就可以邀请伙伴。',
   action: '打开 AI 服务',
  },
  openInChat: '正在 Chat 中打开这个任务',
  // Cindy newChat.atMention.bot: what choosing a teammate in the composer's @ list does.
  mention: {action: '发消息或委派'},
 },
 en: {
  title: 'OneBot',
  loading: 'Opening OneBot',
  loadFailed: 'OneBot could not open',
  retry: 'Retry',
  teammates: 'Teammates',
  connectAI: {
   title: 'Connect an AI service first',
   description: 'Teammates use the models enabled in AI services. Connect a service and enable a model, then invite a teammate.',
   action: 'Open AI services',
  },
  openInChat: 'Opening this task in Chat',
  mention: {action: 'Message or delegate'},
 },
};
