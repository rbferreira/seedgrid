// The three selects for the external upload (category, type, resolution), read from the tracker's
// PROFILE (ADR-037, #196). Before they were hardcoded lists in TrackerUploadPanel.tsx, with the IDs
// of a site; now each site offers what its own profile declares.
//
// Presentational by design (stateless, no side effects, no fetch): this is what allows testing
// "the panel renders the profile options" via server-side rendering, without the DOM.

import type { TaxonomyAxis, TrackerProfile } from "../../api";
import { AXES, termOptions } from "../../lib/taxonomy";
import { useT } from "../../i18n";

export type TaxonomyValues = { category_id: number; type_id: number; resolution_id: number };

export function TaxonomySelects({
  profile,
  values,
  onChange,
  inputCls,
}: {
  profile: TrackerProfile | undefined;
  values: TaxonomyValues;
  onChange: (axis: TaxonomyAxis, id: number) => void;
  inputCls: string;
}) {
  const t = useT();
  return (
    <div className="grid gap-3 sm:grid-cols-3">
      {AXES.map(({ axis, labelKey }) => {
        const options = termOptions(profile, axis);
        const value = values[`${axis}_id` as keyof TaxonomyValues];
        // A value not in the profile (the profile changed, or the parser suggested something): shows it
        // as out of profile instead of choosing another option behind the scenes.
        const orphan = value > 0 && !options.some((o) => o.id === value);
        return (
          <div key={axis}>
            <label className="mb-1 block text-xs text-slate-400">{t(labelKey)}</label>
            <select
              aria-label={t(labelKey)}
              value={value}
              onChange={(e) => onChange(axis, Number(e.target.value))}
              className={`${inputCls} sg-select`}
            >
              {/* Resolution is optional in UNIT3D; category and type are not, but 0 is also the
                  state "not yet chosen" — better than a default that no one checked. */}
              <option value={0}>{options.length === 0 ? t("publish.taxonomySelects.noOptionsInProfile") : t("publish.taxonomySelects.selectPlaceholder")}</option>
              {orphan && <option value={value}>{t("publish.taxonomySelects.orphanOption", { value })}</option>}
              {options.map((o) => (
                <option key={o.id} value={o.id}>
                  {o.label}
                </option>
              ))}
            </select>
          </div>
        );
      })}
    </div>
  );
}
