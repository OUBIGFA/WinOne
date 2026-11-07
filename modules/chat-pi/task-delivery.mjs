// Cindy 2d8d3c3 distinguishes a finished turn from a newly delivered result.
// The authenticated Host binds this report to the immutable input receipt;
// the model supplies only its judgement, never a session or execution id.
export const TASK_DELIVERY_NOTE = 'Before your final answer, call report_task_delivery with final_delivery:true if this turn newly delivers the requested result (including a PR ready for human review); use false for intermediate work, waiting, or repeated status. This is bookkeeping, not a claim that another task has finished.';

export function taskDeliveryTools(config) {
  if (!config.taskDelivery) return [];
  return [{name: 'report_task_delivery', label: 'Report task delivery', description: TASK_DELIVERY_NOTE,
    parameters: {type: 'object', properties: {final_delivery: {type: 'boolean'}}, required: ['final_delivery'], additionalProperties: false},
    async execute(_id, args, signal) {
      const requestId = await config.taskInputReceipt?.();
      if (!requestId) throw Error('当前任务输入凭据不可用，未记录交付状态');
      const response = await fetch(config.baseUrl + '/task/delivery', {method: 'POST',
        headers: {Authorization: 'Bearer ' + config.key, 'Content-Type': 'application/json'},
        body: JSON.stringify({requestId, finalDelivery: args.final_delivery}), signal});
      if (!response.ok) throw Error(await response.text());
      const value = await response.json();
      if (!value.ok) throw Error(value.message ?? '交付状态未记录');
      return {content: [{type: 'text', text: JSON.stringify(value)}], details: value};
    }}];
}
