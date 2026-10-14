import { useCallback, useEffect, useState } from "react";
import {
  listUsers,
  createUser,
  updateUser,
  deleteUser,
  resetUserPassword,
  getMe,
  type User,
  type Role,
  ApiError,
} from "../api";
import { getLang, useT, type Key, type TFn } from "../i18n";

// System ROLES (ADR-035) — used in create/edit selects.
const ROLES: Role[] = ["Administrator", "Publisher", "Downloader", "Viewer"];
// Account status, how the API saves and displays it (enum value, not UI text).
const USER_STATUSES = ["ACTIVE", "DISABLED", "LOCKED"] as const;

const ROLE_DESC_KEY: Record<string, Key> = {
  Administrator: "users.users.roleDesc.administrator",
  Publisher: "users.users.roleDesc.publisher",
  Downloader: "users.users.roleDesc.downloader",
  Viewer: "users.users.roleDesc.viewer",
};

function roleBadge(role: string, t: TFn) {
  const color =
    role === "Administrator"
      ? "bg-red-500/15 text-red-400 ring-red-500/30"
      : role === "Publisher"
        ? "bg-amber-500/15 text-amber-300 ring-amber-500/30"
        : role === "Downloader"
          ? "bg-sky-500/15 text-sky-300 ring-sky-500/30"
          : "bg-slate-500/15 text-slate-300 ring-slate-500/30";
  return (
    <span
      key={role}
      title={ROLE_DESC_KEY[role] ? t(ROLE_DESC_KEY[role]) : role}
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${color}`}
    >
      {role}
    </span>
  );
}

export function Users() {
  const t = useT();
  const [users, setUsers] = useState<User[]>([]);
  const [me, setMe] = useState<{ username: string } | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  // Creation form.
  const [creating, setCreating] = useState(false);
  const [form, setForm] = useState({ username: "", display_name: "", email: "", password: "", roles: ["Viewer"] as string[] });
  const [saving, setSaving] = useState(false);

  // Inline edit: expands a row to change roles/status/reset.
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editRoles, setEditRoles] = useState<string[]>([]);
  const [editStatus, setEditStatus] = useState("ACTIVE");
  const [newPassword, setNewPassword] = useState("");

  const load = useCallback(async () => {
    try {
      const [us, m] = await Promise.all([listUsers(), getMe()]);
      setUsers(us);
      setMe(m);
      setError("");
    } catch (err) {
      setError(t("users.users.error.load"));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    load();
  }, [load]);

  async function submitCreate() {
    setError("");
    setNotice("");
    if (form.username.trim() === "" || form.password.length < 6) {
      setError(t("users.users.error.createValidation"));
      return;
    }
    setSaving(true);
    try {
      await createUser({
        username: form.username.trim(),
        display_name: form.display_name.trim() || undefined,
        email: form.email.trim() || undefined,
        password: form.password,
        roles: form.roles,
      });
      setForm({ username: "", display_name: "", email: "", password: "", roles: ["Viewer"] });
      setCreating(false);
      setNotice(t("users.users.notice.created"));
      await load();
    } catch (err) {
      if (err instanceof ApiError) setError(err.message);
      else setError(t("users.users.error.create"));
    } finally {
      setSaving(false);
    }
  }

  function openEdit(u: User) {
    setEditingId(u.id);
    setEditRoles([...u.roles]);
    setEditStatus(u.status);
    setNewPassword("");
    setError("");
    setNotice("");
  }

  async function saveEdit(u: User) {
    setError("");
    setNotice("");
    try {
      await updateUser(u.id, { roles: editRoles, status: editStatus });
      if (newPassword.length > 0) {
        await resetUserPassword(u.id, newPassword);
      }
      setEditingId(null);
      setNotice(t("users.users.notice.updated", { username: u.username }));
      await load();
    } catch (err) {
      if (err instanceof ApiError) setError(err.message);
      else setError(t("users.users.error.update"));
    }
  }

  async function remove(u: User) {
    if (!window.confirm(t("users.users.confirm.disable", { username: u.username }))) return;
    setError("");
    setNotice("");
    try {
      await deleteUser(u.id);
      setNotice(t("users.users.notice.disabled", { username: u.username }));
      await load();
    } catch (err) {
      if (err instanceof ApiError) setError(err.message);
      else setError(t("users.users.error.disable"));
    }
  }

  if (loading) return <p className="text-slate-400">{t("users.users.loading")}</p>;

  return (
    <>
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-lg font-semibold text-white">{t("users.users.title")}</h1>
        <button
          onClick={() => setCreating((v) => !v)}
          className="rounded-full bg-emerald-600 px-3 py-1.5 text-sm font-medium text-white transition hover:bg-emerald-500"
        >
          {creating ? t("common.cancel") : t("users.users.actions.new")}
        </button>
      </div>

      {error && <p className="rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</p>}
      {notice && <p className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 px-3 py-2 text-sm text-emerald-400">{notice}</p>}

      {creating && (
        <div className="space-y-3 rounded-2xl border border-slate-800 bg-slate-900/60 p-5">
          <h2 className="text-sm font-semibold text-white">{t("users.users.form.title")}</h2>
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <label className="mb-1 block text-sm text-slate-300">{t("users.users.form.username")}</label>
              <input
                value={form.username}
                onChange={(e) => setForm({ ...form, username: e.target.value })}
                className="sg-field w-full"
              />
            </div>
            <div>
              <label className="mb-1 block text-sm text-slate-300">{t("users.users.form.displayName")}</label>
              <input
                value={form.display_name}
                onChange={(e) => setForm({ ...form, display_name: e.target.value })}
                className="sg-field w-full"
              />
            </div>
            <div>
              <label className="mb-1 block text-sm text-slate-300">{t("users.users.form.email")}</label>
              <input
                value={form.email}
                onChange={(e) => setForm({ ...form, email: e.target.value })}
                className="sg-field w-full"
              />
            </div>
            <div>
              <label className="mb-1 block text-sm text-slate-300">{t("users.users.form.password")}</label>
              <input
                type="password"
                value={form.password}
                onChange={(e) => setForm({ ...form, password: e.target.value })}
                className="sg-field w-full"
              />
            </div>
            <div className="sm:col-span-2">
              <label className="mb-1 block text-sm text-slate-300">{t("users.users.form.roles")}</label>
              <div className="flex flex-wrap gap-2">
                {ROLES.map((r) => (
                  <label key={r} className="flex cursor-pointer items-center gap-1.5 text-sm text-slate-200">
                    <input
                      type="checkbox"
                      checked={form.roles.includes(r)}
                      onChange={(e) =>
                        setForm({
                          ...form,
                          roles: e.target.checked
                            ? [...form.roles, r]
                            : form.roles.filter((x) => x !== r),
                        })
                      }
                      className="sg-check"
                    />
                    {r}
                  </label>
                ))}
              </div>
              <p className="mt-1 text-xs text-slate-500">{t("users.users.form.rolesHint")}</p>
            </div>
          </div>
          <button
            onClick={submitCreate}
            disabled={saving}
            className="rounded-full bg-emerald-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-emerald-500 disabled:opacity-50"
          >
            {saving ? t("users.users.actions.creating") : t("users.users.actions.create")}
          </button>
        </div>
      )}

      <div className="overflow-x-auto rounded-2xl border border-slate-800">
        <table className="w-full min-w-[760px] text-left text-sm">
          <thead className="border-b border-slate-800 bg-slate-900/60 text-xs uppercase tracking-wide text-slate-500">
            <tr>
              <th className="px-4 py-2.5">{t("users.users.table.user")}</th>
              <th className="px-4 py-2.5">{t("users.users.table.roles")}</th>
              <th className="px-4 py-2.5">{t("users.users.table.status")}</th>
              <th className="px-4 py-2.5">{t("users.users.table.lastLogin")}</th>
              <th className="px-4 py-2.5" />
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-800/70">
            {users.map((u) => (
              <tr key={u.id} className="align-top">
                <td className="px-4 py-3">
                  <div className="font-medium text-white">{u.username}</div>
                  <div className="text-xs text-slate-500">
                    {u.display_name || "—"}
                    {u.email ? ` · ${u.email}` : ""}
                  </div>
                </td>
                <td className="px-4 py-3">
                  {editingId === u.id ? (
                    <div className="flex max-w-[260px] flex-wrap gap-1.5">
                      {ROLES.map((r) => (
                        <label key={r} className="flex cursor-pointer items-center gap-1 text-xs text-slate-200">
                          <input
                            type="checkbox"
                            checked={editRoles.includes(r)}
                            onChange={(e) =>
                              setEditRoles(
                                e.target.checked ? [...editRoles, r] : editRoles.filter((x) => x !== r),
                              )
                            }
                            className="sg-check"
                          />
                          {r}
                        </label>
                      ))}
                    </div>
                  ) : (
                    <div className="flex flex-wrap gap-1.5">{u.roles.map((r) => roleBadge(r, t))}</div>
                  )}
                </td>
                <td className="px-4 py-3">
                  {editingId === u.id ? (
                    <select
                      value={editStatus}
                      onChange={(e) => setEditStatus(e.target.value)}
                      className="sg-field-sm sg-select"
                    >
                      {USER_STATUSES.map((st) => (
                        <option key={st} value={st}>
                          {st}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <span
                      className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${
                        u.status === "ACTIVE"
                          ? "bg-emerald-500/15 text-emerald-400"
                          : "bg-red-500/15 text-red-400"
                      }`}
                    >
                      {u.status}
                    </span>
                  )}
                </td>
                <td className="px-4 py-3 text-xs text-slate-500">
                  {u.last_login_at ? new Date(u.last_login_at).toLocaleString(getLang()) : t("users.users.table.never")}
                </td>
                <td className="px-4 py-3 text-right">
                  {editingId === u.id ? (
                    <div className="flex flex-col items-end gap-1.5">
                      <input
                        type="password"
                        placeholder={t("users.users.edit.passwordPlaceholder")}
                        value={newPassword}
                        onChange={(e) => setNewPassword(e.target.value)}
                        className="sg-field-sm w-44"
                      />
                      <div className="flex gap-1.5">
                        <button
                          onClick={() => saveEdit(u)}
                          className="rounded-full bg-emerald-600 px-2.5 py-1 text-xs font-medium text-white hover:bg-emerald-500"
                        >
                          {t("common.save")}
                        </button>
                        <button
                          onClick={() => setEditingId(null)}
                          className="rounded-full bg-slate-700 px-2.5 py-1 text-xs text-slate-200 hover:bg-slate-600"
                        >
                          {t("common.cancel")}
                        </button>
                      </div>
                    </div>
                  ) : (
                    <div className="flex justify-end gap-1.5">
                      {me && u.username !== me.username && u.status === "ACTIVE" && (
                        <button
                          onClick={() => remove(u)}
                          className="rounded-full bg-red-500/15 px-2.5 py-1 text-xs font-medium text-red-400 ring-1 ring-inset ring-red-500/30 transition hover:ring-red-500/70"
                        >
                          {t("users.users.actions.disable")}
                        </button>
                      )}
                      <button
                        onClick={() => openEdit(u)}
                        className="rounded-full bg-slate-700/60 px-2.5 py-1 text-xs text-slate-200 hover:bg-slate-600"
                      >
                        {t("users.users.actions.edit")}
                      </button>
                    </div>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="text-xs text-slate-500">
        {t("users.users.footer.roles", { roles: ROLES.map((r) => `${r} — ${t(ROLE_DESC_KEY[r])}`).join(" · ") })}
      </p>
    </>
  );
}
