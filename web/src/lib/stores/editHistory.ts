import { api } from '$lib/api';

export type Revision = {
  body: string;
  at?: string;
  original: boolean;
};

export async function fetchEditHistory(channelId: string, archiveId: string): Promise<Revision[]> {
  return api<Revision[]>(
    `/api/channels/${channelId}/messages/${encodeURIComponent(archiveId)}/history`
  );
}
