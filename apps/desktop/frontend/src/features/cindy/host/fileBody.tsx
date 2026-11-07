// WinOne seam for Cindy workdir-browse FileBodyView and hooks/useFileContent
// as the attachment preview (FileLightbox, 2d8d3c3) uses them. The file body
// is WinOne Chat's own preview (text, image, PDF, audio/video) read through
// the Host for the session; nothing is edited here, as upstream passes
// allowEdit={false}. Not verified against a OneBot teammate session yet: if
// the Host refuses the session, the preview shows its error and retry.
import {ChatFilePreview} from '../../chat-file-preview';

type FileContent = {kind: 'loading'};

/** Cindy reads the file once to size the window; the WinOne preview reads it itself. */
export function useFileContent(_workdir: string, _relPath: string): {content: FileContent} {
  return {content: {kind: 'loading'}};
}

export function FileBodyView({workdir, relPath, sessionId}: {workdir: string; relPath: string; content?: unknown; sessionId?: string; allowEdit?: boolean}) {
  const path = workdir.replace(/[\\/]+$/, '') + (workdir.includes('/') && !workdir.includes('\\') ? '/' : '\\') + relPath;
  return <div className="h-full overflow-hidden">
    <ChatFilePreview id={sessionId ?? ''} path={path} onDirty={() => {}} onHandle={() => {}}/>
  </div>;
}
