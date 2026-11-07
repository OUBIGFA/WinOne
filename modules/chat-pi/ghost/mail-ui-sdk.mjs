// Native injection bundle: the mail document's CSP forbids scripts. Only the
// trusted MyGo host evaluates these locked Cindy helpers; no app/Go authority
// or credentials are exported to the email's own HTML.
import {mailBodySerializer} from '../cindy/ghost/apps/desktop/src/renderer/components/chat/mailBodyEditing.ts';
import {followMailPreviewTheme,restoreMailPreviewPaint,mailPreviewLinkUrl} from '../cindy/ghost/apps/desktop/src/renderer/components/chat/mailPreviewAppearance.ts';
import {mailInlineDocument,mailReadingDocument,safeMailDocument,mailTextParagraphs,hasMailExternalImages,externalMailImageUrl,mailInlineRefreshScript,mailReadingThemeScript} from '../cindy/ghost/packages/maker-shared/src/mailTextEdit.ts';

globalThis.WinOneMail={mailBodySerializer,followMailPreviewTheme,restoreMailPreviewPaint,mailPreviewLinkUrl,mailInlineDocument,mailReadingDocument,safeMailDocument,mailTextParagraphs,hasMailExternalImages,externalMailImageUrl,mailInlineRefreshScript,mailReadingThemeScript};
