import { api } from '$lib/api';

export type Task = {
  id: string;
  space_id: string;
  channel_id?: string;
  source_archive_id?: string;
  title: string;
  assignee_id?: string;
  due_at?: string;
  status: 'open' | 'done';
  created_at: string;
};

export async function listTasks(spaceId: string): Promise<Task[]> {
  return api<Task[]>(`/api/spaces/${spaceId}/tasks`);
}

export async function createTask(
  spaceId: string,
  title: string,
  extra: { source_archive_id?: string; channel_id?: string } = {}
): Promise<Task> {
  return api<Task>(`/api/spaces/${spaceId}/tasks`, { method: 'POST', body: { title, ...extra } });
}

export async function setTaskStatus(taskId: string, status: 'open' | 'done'): Promise<Task> {
  return api<Task>(`/api/tasks/${taskId}`, { method: 'PATCH', body: { status } });
}

export async function deleteTask(taskId: string): Promise<void> {
  await api(`/api/tasks/${taskId}`, { method: 'DELETE' });
}
