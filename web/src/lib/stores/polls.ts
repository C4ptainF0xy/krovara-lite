import { writable, get } from 'svelte/store';
import { api } from '$lib/api';

export type PollOption = { id: string; label: string; votes: number };
export type Poll = {
  id: string;
  channel_id: string;
  question: string;
  created_by: string;
  closed: boolean;
  created_at: string;
  options: PollOption[];
  my_option: string | null;
};

export const pollsByChannel = writable<Record<string, Poll[]>>({});

export async function loadPolls(channelId: string): Promise<Poll[]> {
  if (!channelId) return [];
  const data = await api<Poll[]>(`/api/channels/${channelId}/polls`);
  pollsByChannel.update((m) => ({ ...m, [channelId]: data }));
  return data;
}

export async function createPoll(channelId: string, question: string, options: string[]): Promise<void> {
  await api(`/api/channels/${channelId}/polls`, { method: 'POST', body: { question, options } });
  await loadPolls(channelId);
}

export async function votePoll(channelId: string, pollId: string, optionId: string): Promise<void> {
  await api(`/api/polls/${pollId}/vote`, { method: 'POST', body: { option_id: optionId } });
  await loadPolls(channelId);
}

export async function closePoll(channelId: string, pollId: string): Promise<void> {
  await api(`/api/polls/${pollId}/close`, { method: 'POST' });
  await loadPolls(channelId);
}

export function totalVotes(p: Poll): number {
  return p.options.reduce((n, o) => n + o.votes, 0);
}

export function pollCount(channelId: string): number {
  return (get(pollsByChannel)[channelId] ?? []).length;
}
