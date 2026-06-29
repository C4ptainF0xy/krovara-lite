import { api } from '$lib/api';

export type Listing = {
  space_id: string;
  name: string;
  description: string | null;
  icon_key: string | null;
  banner_key: string | null;
  category: string;
  member_count: number;
  tags: string[];
  language: string | null;
  vanity_slug: string | null;
};

export type ListingState = { listed: boolean; category?: string; member_count?: number };

export const CATEGORIES = ['gaming', 'tech', 'art', 'music', 'education', 'community', 'other'];

export async function explore(category = '', q = ''): Promise<Listing[]> {
  const params = new URLSearchParams();
  if (category) params.set('category', category);
  if (q) params.set('q', q);
  const qs = params.toString();
  return api<Listing[]>(`/api/discover${qs ? `?${qs}` : ''}`);
}

export async function getListing(spaceId: string): Promise<ListingState> {
  return api<ListingState>(`/api/spaces/${spaceId}/listing`);
}

export async function listSpace(spaceId: string, category: string): Promise<ListingState> {
  return api<ListingState>(`/api/spaces/${spaceId}/listing`, { method: 'PUT', body: { category } });
}

export async function delistSpace(spaceId: string): Promise<void> {
  await api(`/api/spaces/${spaceId}/listing`, { method: 'DELETE' });
}

export type OpenJoinResult = { space_id: string };

export async function openJoin(spaceId: string): Promise<OpenJoinResult> {
  return api<OpenJoinResult>(`/api/discover/${spaceId}/join`, { method: 'POST' });
}
