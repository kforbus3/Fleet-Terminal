import { api } from "./client";

// Typed wrappers over the admin surface (/users /roles /groups /permissions
// /settings). Response envelopes mirror the backend's map[string]any shapes.

export interface User {
  id: string;
  username: string;
  email?: string;
  displayName: string;
  isSuperAdmin: boolean;
  isDisabled: boolean;
  emailVerified: boolean;
  mustChangePassword: boolean;
  lockedUntil?: string;
  lastLoginAt?: string;
  createdAt: string;
  updatedAt: string;
  roles?: string[];
  groups?: string[];
}

export interface Role {
  id: string;
  name: string;
  description: string;
  isBuiltin: boolean;
  createdAt: string;
  permissions?: string[];
}

export interface Permission {
  key: string;
  description: string;
}

export interface Group {
  id: string;
  name: string;
  description: string;
  createdAt: string;
}

export interface CreateUserInput {
  username: string;
  email: string;
  displayName: string;
  password: string;
  isSuperAdmin: boolean;
  mustChangePassword: boolean;
}

export interface UpdateUserInput {
  email: string;
  displayName: string;
  isDisabled: boolean;
}

export async function listUsers(): Promise<User[]> {
  const { data } = await api.get<{ users: User[] }>("/api/v1/users");
  return data.users;
}

export async function createUser(input: CreateUserInput): Promise<User> {
  const { data } = await api.post<User>("/api/v1/users", input);
  return data;
}

export async function updateUser(id: string, input: UpdateUserInput): Promise<void> {
  await api.put(`/api/v1/users/${id}`, input);
}

export async function deleteUser(id: string): Promise<void> {
  await api.delete(`/api/v1/users/${id}`);
}

export async function listRoles(): Promise<Role[]> {
  const { data } = await api.get<{ roles: Role[] }>("/api/v1/roles");
  return data.roles;
}

export async function createRole(name: string, description: string): Promise<Role> {
  const { data } = await api.post<Role>("/api/v1/roles", { name, description });
  return data;
}

export async function deleteRole(id: string): Promise<void> {
  await api.delete(`/api/v1/roles/${id}`);
}

export async function setRolePermissions(id: string, permissions: string[]): Promise<void> {
  await api.put(`/api/v1/roles/${id}/permissions`, { permissions });
}

export async function listPermissions(): Promise<Permission[]> {
  const { data } = await api.get<{ permissions: Permission[] }>("/api/v1/permissions");
  return data.permissions;
}

export async function listGroups(): Promise<Group[]> {
  const { data } = await api.get<{ groups: Group[] }>("/api/v1/groups");
  return data.groups;
}

export async function createGroup(name: string, description: string): Promise<Group> {
  const { data } = await api.post<Group>("/api/v1/groups", { name, description });
  return data;
}

export async function deleteGroup(id: string): Promise<void> {
  await api.delete(`/api/v1/groups/${id}`);
}

export type Settings = Record<string, unknown>;

export async function listSettings(): Promise<Settings> {
  const { data } = await api.get<{ settings: Settings }>("/api/v1/settings");
  return data.settings ?? {};
}

export async function setSetting(key: string, value: unknown): Promise<void> {
  await api.put(`/api/v1/settings/${encodeURIComponent(key)}`, value);
}
