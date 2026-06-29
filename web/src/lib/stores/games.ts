import { api } from '$lib/api';

export type Game = {
  id: string;
  name: string;
  cover_key: string | null;
  status: 'pending' | 'approved' | 'rejected';
  aliases: string[];
  reject_reason?: string;
};

export async function listGames(q = ''): Promise<Game[]> {
  const qs = q ? `?q=${encodeURIComponent(q)}` : '';
  return api<Game[]>(`/api/games${qs}`);
}

export async function submitGame(
  name: string,
  aliases: string[] = [],
  coverKey: string | null = null
): Promise<Game> {
  return api<Game>('/api/games', { method: 'POST', body: { name, aliases, cover_key: coverKey } });
}

export async function listPendingGames(): Promise<Game[]> {
  return api<Game[]>('/api/admin/games/pending');
}

export async function reviewGame(id: string, approve: boolean, reason = ''): Promise<Game> {
  return api<Game>(`/api/admin/games/${id}/review`, { method: 'POST', body: { approve, reason } });
}
