import { writable } from 'svelte/store';
import { api } from '$lib/api';

export const pendingReports = writable<Record<string, number>>({});

export async function refreshPendingReports(spaceId: string): Promise<void> {
  try {
    const res = await api<{ pending: number }>(`/api/spaces/${spaceId}/reports/count`);
    pendingReports.update((m) => ({ ...m, [spaceId]: res.pending }));
  } catch {
  }
}

export async function claimReport(spaceId: string, reportId: string): Promise<void> {
  await api(`/api/spaces/${spaceId}/reports/${reportId}/claim`, { method: 'POST' });
}

export type ReportComment = {
  id: string;
  report_id: string;
  author_id?: string;
  body: string;
  created_at: string;
};

export async function listReportComments(
  spaceId: string,
  reportId: string
): Promise<ReportComment[]> {
  return api<ReportComment[]>(`/api/spaces/${spaceId}/reports/${reportId}/comments`);
}

export async function addReportComment(
  spaceId: string,
  reportId: string,
  body: string
): Promise<ReportComment> {
  return api<ReportComment>(`/api/spaces/${spaceId}/reports/${reportId}/comments`, {
    method: 'POST',
    body: { body }
  });
}
