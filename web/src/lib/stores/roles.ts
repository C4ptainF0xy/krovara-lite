import { writable, get } from 'svelte/store';
import { api } from '$lib/api';

export type Role = {
  id: string;
  space_id: string;
  name: string;
  permissions: number | null;
  color: string | null;
  position: number | null;
  is_everyone: boolean | null;
  hoist?: boolean;
  mentionable?: boolean;
  icon_emoji?: string | null;
};

export type RoleMember = {
  member_id: string;
  user_id: string;
  username: string;
  nickname: string | null;
};

export const rolesBySpace = writable<Record<string, Role[]>>({});

export async function loadRoles(spaceId: string): Promise<Role[]> {
  const data = await api<Role[]>(`/api/spaces/${spaceId}/roles`);
  data.sort((a, b) => (b.position ?? 0) - (a.position ?? 0));
  rolesBySpace.update((m) => ({ ...m, [spaceId]: data }));
  return data;
}

export function rolesFor(spaceId: string): Role[] {
  return get(rolesBySpace)[spaceId] ?? [];
}

export async function createRole(spaceId: string, name: string): Promise<Role> {
  const existing = rolesFor(spaceId);
  const maxNonEveryone = existing
    .filter((r) => !r.is_everyone)
    .reduce((mx, r) => Math.max(mx, r.position ?? 0), 0);
  const created = await api<Role>(`/api/spaces/${spaceId}/roles`, {
    method: 'POST',
    body: { name, position: maxNonEveryone + 1 }
  });
  await loadRoles(spaceId);
  return created;
}

export type RolePatch = {
  name?: string;
  permissions?: number;
  color?: string | null;
  position?: number;
  hoist?: boolean;
  mentionable?: boolean;
  icon_emoji?: string | null;
};

export async function updateRole(spaceId: string, roleId: string, patch: RolePatch): Promise<Role> {
  const updated = await api<Role>(`/api/roles/${roleId}`, { method: 'PATCH', body: patch });
  rolesBySpace.update((m) => ({
    ...m,
    [spaceId]: (m[spaceId] ?? []).map((r) => (r.id === roleId ? updated : r))
  }));
  return updated;
}

export async function deleteRole(spaceId: string, roleId: string): Promise<void> {
  await api(`/api/roles/${roleId}`, { method: 'DELETE' });
  rolesBySpace.update((m) => ({
    ...m,
    [spaceId]: (m[spaceId] ?? []).filter((r) => r.id !== roleId)
  }));
}

export async function listRoleMembers(roleId: string): Promise<RoleMember[]> {
  return api<RoleMember[]>(`/api/roles/${roleId}/members`);
}

export async function memberRoleIds(memberId: string): Promise<string[]> {
  const res = await api<{ role_ids: string[] }>(`/api/members/${memberId}/roles`);
  return res.role_ids ?? [];
}

export async function assignRole(memberId: string, roleId: string): Promise<void> {
  await api(`/api/members/${memberId}/roles/${roleId}`, { method: 'PUT' });
}
export async function removeRole(memberId: string, roleId: string): Promise<void> {
  await api(`/api/members/${memberId}/roles/${roleId}`, { method: 'DELETE' });
}

export async function bulkAssign(
  roleId: string,
  action: 'add' | 'remove',
  opts: { memberIds?: string[]; everyone?: boolean; expiresInSeconds?: number }
): Promise<number> {
  const res = await api<{ affected: number }>(`/api/roles/${roleId}/members`, {
    method: 'POST',
    body: {
      action,
      member_ids: opts.memberIds ?? [],
      everyone: !!opts.everyone,
      expires_in_seconds: opts.expiresInSeconds ?? 0
    }
  });
  return res.affected;
}

export async function reorderRoles(spaceId: string, orderedIds: string[]): Promise<void> {
  const roles = rolesFor(spaceId);
  const n = orderedIds.length;
  await Promise.all(
    orderedIds.map((id, i) => {
      const role = roles.find((r) => r.id === id);
      if (!role || role.is_everyone) return Promise.resolve();
      const pos = n - i;
      if ((role.position ?? 0) === pos) return Promise.resolve();
      return updateRole(spaceId, id, { position: pos }).catch(() => {});
    })
  );
  await loadRoles(spaceId);
}
