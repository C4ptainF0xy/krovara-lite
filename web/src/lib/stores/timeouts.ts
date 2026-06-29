import { api } from '$lib/api';

export type Timeout = {
  active: boolean;
  expires_at?: string;
  reason?: string | null;
};

export async function timeoutMember(
  spaceId: string,
  userId: string,
  minutes: number,
  reason?: string
): Promise<void> {
  await api(`/api/spaces/${spaceId}/timeouts`, {
    method: 'POST',
    body: { user_id: userId, minutes, reason: reason ?? '' }
  });
}

export async function liftTimeout(spaceId: string, userId: string): Promise<void> {
  await api(`/api/spaces/${spaceId}/timeouts/${userId}`, { method: 'DELETE' });
}

export async function myTimeout(spaceId: string): Promise<Timeout> {
  return api<Timeout>(`/api/spaces/${spaceId}/timeouts/me`);
}

export async function memberTimeout(spaceId: string, userId: string): Promise<Timeout> {
  return api<Timeout>(`/api/spaces/${spaceId}/timeouts/${userId}`);
}
