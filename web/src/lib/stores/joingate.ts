import { api } from '$lib/api';

export type JoinQuestion = {
  id: string;
  label: string;
  required: boolean;
};

export type JoinForm = {
  enabled: boolean;
  questions: JoinQuestion[];
  auto_role_id: string | null;
  min_karma: number;
};

export type JoinAnswer = {
  question_id: string;
  answer: string;
};

export type JoinRequest = {
  id: string;
  user_id: string;
  username: string;
  display_name: string | null;
  avatar_key: string | null;
  answers: JoinAnswer[];
  status: string;
  created_at: string;
};

export function getJoinForm(spaceId: string): Promise<JoinForm> {
  return api<JoinForm>(`/api/spaces/${spaceId}/join-form`);
}

export function saveJoinForm(spaceId: string, form: JoinForm): Promise<JoinForm> {
  return api<JoinForm>(`/api/spaces/${spaceId}/join-form`, { method: 'PUT', body: form });
}

export function submitJoinRequest(spaceId: string, answers: JoinAnswer[]): Promise<JoinRequest> {
  return api<JoinRequest>(`/api/spaces/${spaceId}/join-requests`, {
    method: 'POST',
    body: { answers }
  });
}

export function listJoinRequests(spaceId: string, status = 'pending'): Promise<JoinRequest[]> {
  return api<JoinRequest[]>(`/api/spaces/${spaceId}/join-requests?status=${status}`);
}

export function reviewJoinRequest(
  requestId: string,
  action: 'approve' | 'reject'
): Promise<{ id: string; status: string; member_created: boolean }> {
  return api(`/api/join-requests/${requestId}/review`, { method: 'POST', body: { action } });
}
