import { api } from '$lib/api';

type KarmaResp = { user_id: string; space_id: string; score: number };

export function getKarma(spaceId: string, userId: string): Promise<number> {
  return api<KarmaResp>(`/api/spaces/${spaceId}/karma/${userId}`).then((r) => r.score);
}

export function vouch(spaceId: string, userId: string): Promise<number> {
  return api<KarmaResp>(`/api/spaces/${spaceId}/karma/${userId}`, { method: 'POST' }).then(
    (r) => r.score
  );
}
