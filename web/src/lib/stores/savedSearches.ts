import { api } from '$lib/api';

export type SavedSearch = {
  id: string;
  name: string;
  query: string;
  space_id: string | null;
  created_at: string;
};

export function listSavedSearches(): Promise<SavedSearch[]> {
  return api<SavedSearch[]>('/api/me/saved-searches');
}

export function createSavedSearch(
  name: string,
  query: string,
  spaceId?: string | null
): Promise<SavedSearch> {
  return api<SavedSearch>('/api/me/saved-searches', {
    method: 'POST',
    body: { name, query, space_id: spaceId ?? null }
  });
}

export function deleteSavedSearch(id: string): Promise<void> {
  return api(`/api/me/saved-searches/${id}`, { method: 'DELETE' });
}
