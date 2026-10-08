import type { Catalog } from "../types";

export const profile: Catalog["profile"] = {
  title: "Profile",
  operator: "Operator",
  placeholder: "profile",
  changePassword: {
    title: "Change password",
    current: "Current password",
    next: "New password",
    confirm: "Confirm new password",
    submit: "Change password",
    submitting: "Saving...",
    tooShort: "The new password must be at least 6 characters long.",
    mismatch: "Confirmation does not match the new password.",
    badCurrent: "Incorrect current password or invalid new password.",
    failed: "Failed to change password.",
    success: "✓ Password changed successfully.",
  },
};
