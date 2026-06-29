import { writable } from 'svelte/store';
import { api } from '$lib/api';

export type RsvpStatus = 'going' | 'maybe' | 'no';
export type SpaceEvent = {
  id: string;
  space_id: string;
  title: string;
  description?: string;
  location?: string;
  starts_at: string;
  created_by: string;
  created_at: string;
  rsvp: Record<RsvpStatus, number>;
  my_rsvp: '' | RsvpStatus;
};

export const eventsBySpace = writable<Record<string, SpaceEvent[]>>({});

export async function loadEvents(spaceId: string): Promise<SpaceEvent[]> {
  if (!spaceId) return [];
  const data = await api<SpaceEvent[]>(`/api/spaces/${spaceId}/events`);
  eventsBySpace.update((m) => ({ ...m, [spaceId]: data }));
  return data;
}

export async function createEvent(
  spaceId: string,
  body: { title: string; description?: string | null; location?: string | null; starts_at: string }
): Promise<void> {
  await api(`/api/spaces/${spaceId}/events`, { method: 'POST', body });
  await loadEvents(spaceId);
}

export async function rsvpEvent(spaceId: string, eventId: string, status: RsvpStatus): Promise<void> {
  await api(`/api/events/${eventId}/rsvp`, { method: 'POST', body: { status } });
  await loadEvents(spaceId);
}

export async function deleteEvent(spaceId: string, eventId: string): Promise<void> {
  await api(`/api/events/${eventId}`, { method: 'DELETE' });
  await loadEvents(spaceId);
}
