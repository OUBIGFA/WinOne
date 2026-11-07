/** Compatibility view for ghost_forge_guide. Authoring docs live only in the built-in Skill.
 * Raw imports embed the same shipped Markdown in dev, tests and packaged builds.
 */
import overview from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-overview.md' with { type: 'text' };
import sourceVerification from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-source-verification.md' with { type: 'text' };
import routineEvents from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-routine-events.md' with { type: 'text' };
import design from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-design.md' with { type: 'text' };
import structure from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-structure.md' with { type: 'text' };
import manifest from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-manifest.md' with { type: 'text' };
import tools from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-tools.md' with { type: 'text' };
import toolDiscovery from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-tool-discovery.md' with { type: 'text' };
import manual from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-manual.md' with { type: 'text' };
import runtime from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-runtime.md' with { type: 'text' };
import agentModels from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-agent-models.md' with { type: 'text' };
import media from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-media.md' with { type: 'text' };
import hostContext from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-host-context.md' with { type: 'text' };
import agentCallContext from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-agent-call-context.md' with { type: 'text' };
import chatCards from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-chat-cards.md' with { type: 'text' };
import eventSubscriptions from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-event-subscriptions.md' with { type: 'text' };
import messageHooks from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-message-hooks.md' with { type: 'text' };
import controlPoints from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-control-points.md' with { type: 'text' };
import pluginModels from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-models.md' with { type: 'text' };
import network from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-network.md' with { type: 'text' };
import settings from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-settings.md' with { type: 'text' };
import notifications from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-notifications.md' with { type: 'text' };
import filesAndLibrary from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-files-and-library.md' with { type: 'text' };
import agentTasks from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-agent-tasks.md' with { type: 'text' };
import interactivePages from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-interactive-pages.md' with { type: 'text' };
import nodeWorker from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-node-worker.md' with { type: 'text' };
import sessionContext from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-session-context.md' with { type: 'text' };
import directoryPicker from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-directory-picker.md' with { type: 'text' };
import panelPreview from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-panel-preview.md' with { type: 'text' };
import browserExtension from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-browser-extension.md' with { type: 'text' };
import bundledSkills from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-bundled-skills.md' with { type: 'text' };
import workspaceSessions from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-workspace-sessions.md' with { type: 'text' };
import confirmDialog from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-confirm-dialog.md' with { type: 'text' };
import mainView from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-main-view.md' with { type: 'text' };
import taskView from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-task-view.md' with { type: 'text' };
import recommendations from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-recommendations.md' with { type: 'text' };
import panels from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-panels.md' with { type: 'text' };
import mobilePages from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-mobile-pages.md' with { type: 'text' };
import sandbox from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-sandbox.md' with { type: 'text' };
import packaging from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-packaging.md' with { type: 'text' };
import publishing from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-publishing.md' with { type: 'text' };
import compatibility from '../../../resources/system-skills/cindy-plugin-creator/references/plugin-compatibility.md' with { type: 'text' };
import pageGeneration from '../../../resources/system-skills/cindy-plugin-creator/references/examples/interactive-pages/PAGE-GENERATION.md' with { type: 'text' };

export const FORGE_GUIDE = [
  overview,
  sourceVerification,
  routineEvents,
  design,
  structure,
  manifest,
  tools,
  toolDiscovery,
  manual,
  runtime,
  agentModels,
  media,
  hostContext,
  agentCallContext,
  chatCards,
  eventSubscriptions,
  messageHooks,
  controlPoints,
  pluginModels,
  network,
  settings,
  notifications,
  filesAndLibrary,
  agentTasks,
  nodeWorker,
  sessionContext,
  directoryPicker,
  panelPreview,
  browserExtension,
  bundledSkills,
  workspaceSessions,
  confirmDialog,
  mainView,
  taskView,
  recommendations,
  interactivePages,
  panels,
  mobilePages,
  sandbox,
  packaging,
  publishing,
  compatibility,
].join('\n');

/** ghost_forge_guide section "pages": the same PAGE-GENERATION.md the MCP Apps manual header hashes. */
export const FORGE_PAGE_MANUAL = pageGeneration;
