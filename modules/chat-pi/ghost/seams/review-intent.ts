// Reuse the same authenticated-owner projection as Cindy's native task host.
// This seam supplies only the export boundary; no text parser is duplicated.
import {createAutoReviewIntentProjection} from '../../cindy/tasks/core/autoReviewIntent.ts';
export const restoreAutoReviewUserIntent=createAutoReviewIntentProjection().restore;
