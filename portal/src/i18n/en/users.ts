import type { Catalog } from "../types";

export const users: Catalog["users"] = {
  users: {
    title: "Users",
    loading: "Loading users…",
    roleDesc: {
      administrator: "Full access",
      publisher: "Manages content (torrents, trackers, publishing)",
      downloader: "Reads and downloads content",
      viewer: "Read-only",
    },
    actions: {
      new: "＋ New user",
      create: "Create user",
      creating: "Creating...",
      edit: "Edit",
      disable: "Disable",
    },
    form: {
      title: "New operator",
      username: "Username *",
      displayName: "Display name",
      email: "Email",
      password: "Initial password *",
      roles: "Roles",
      rolesHint: "Tip: an operator with multiple roles accumulates permissions.",
    },
    table: {
      user: "User",
      roles: "Roles",
      status: "Status",
      lastLogin: "Last login",
      never: "never",
    },
    edit: {
      passwordPlaceholder: "New password (optional)",
    },
    confirm: {
      disable: "Disable user “{username}”? They will lose access immediately.",
    },
    notice: {
      created: "✓ User created.",
      updated: "✓ User “{username}” updated.",
      disabled: "✓ User “{username}” disabled.",
    },
    error: {
      load: "Failed to load users.",
      createValidation: "Username is required and password must be at least 6 characters.",
      create: "Failed to create user.",
      update: "Failed to update user.",
      disable: "Failed to disable user.",
    },
    footer: {
      roles: "System roles (ADR-035): {roles}",
    },
  },
};
