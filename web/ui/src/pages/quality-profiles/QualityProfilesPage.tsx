import { useState, useMemo } from "react";
import { Plus, Trash2, Pencil, X } from "lucide-react";
import PageHeader from "@/components/PageHeader";
import Modal from "@beacon-shared/Modal";
import { useConfirm } from "@beacon-shared/ConfirmDialog";
import { toast } from "sonner";
import {
  useQualityProfiles,
  useCreateQualityProfile,
  useUpdateQualityProfile,
  useDeleteQualityProfile,
  type QualityProfile,
  type Quality,
} from "@/api/quality-profiles";
import { card } from "@/lib/styles";

// QUALITY_TIERS is the set of quality definition slugs that both Prism and
// Pilot seed in their quality_definitions tables. This is the intersection
// of the two services' definitions, so any profile using these slugs will
// work in both. Pilot seeds all 14; Prism seeds a superset of 29.
const QUALITY_TIERS: Quality[] = [
  { resolution: "sd", source: "dvd", codec: "xvid", hdr: "none", name: "SD DVD" },
  { resolution: "sd", source: "hdtv", codec: "x264", hdr: "none", name: "SD HDTV" },
  { resolution: "720p", source: "hdtv", codec: "x264", hdr: "none", name: "720p HDTV" },
  { resolution: "720p", source: "webdl", codec: "x264", hdr: "none", name: "720p WEBDL" },
  { resolution: "720p", source: "webrip", codec: "x264", hdr: "none", name: "720p WEBRip" },
  { resolution: "720p", source: "bluray", codec: "x264", hdr: "none", name: "720p Bluray" },
  { resolution: "1080p", source: "hdtv", codec: "x264", hdr: "none", name: "1080p HDTV" },
  { resolution: "1080p", source: "webdl", codec: "x264", hdr: "none", name: "1080p WEBDL" },
  { resolution: "1080p", source: "webrip", codec: "x265", hdr: "none", name: "1080p WEBRip" },
  { resolution: "1080p", source: "bluray", codec: "x265", hdr: "none", name: "1080p Bluray" },
  { resolution: "1080p", source: "remux", codec: "x265", hdr: "none", name: "1080p Remux" },
  { resolution: "2160p", source: "webdl", codec: "x265", hdr: "hdr10", name: "2160p WEBDL HDR" },
  { resolution: "2160p", source: "bluray", codec: "x265", hdr: "hdr10", name: "2160p Bluray HDR" },
  { resolution: "2160p", source: "remux", codec: "x265", hdr: "hdr10", name: "2160p Remux HDR" },
];

function qualityKey(q: Quality): string {
  return `${q.resolution}-${q.source}-${q.codec}-${q.hdr}`;
}

interface ProfileFormProps {
  initial?: QualityProfile;
  onClose: () => void;
}

function ProfileForm({ initial, onClose }: ProfileFormProps) {
  const create = useCreateQualityProfile();
  const update = useUpdateQualityProfile();

  const initialQualities: Quality[] = useMemo(() => {
    if (!initial?.qualities_json) return [];
    try {
      return JSON.parse(initial.qualities_json);
    } catch {
      return [];
    }
  }, [initial]);

  const initialCutoff: Quality | null = useMemo(() => {
    if (!initial?.cutoff_json || initial.cutoff_json === "{}") return null;
    try {
      return JSON.parse(initial.cutoff_json);
    } catch {
      return null;
    }
  }, [initial]);

  const [name, setName] = useState(initial?.name ?? "");
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(
    new Set(initialQualities.map(qualityKey)),
  );
  const [cutoffKey, setCutoffKey] = useState<string>(
    initialCutoff ? qualityKey(initialCutoff) : "",
  );
  const [upgradeAllowed, setUpgradeAllowed] = useState(initial?.upgrade_allowed ?? true);

  const isEdit = !!initial;
  const isPending = create.isPending || update.isPending;

  const selectedQualities = QUALITY_TIERS.filter((q) => selectedKeys.has(qualityKey(q)));
  const cutoffQuality = selectedQualities.find((q) => qualityKey(q) === cutoffKey);

  function toggleQuality(q: Quality) {
    const k = qualityKey(q);
    const next = new Set(selectedKeys);
    if (next.has(k)) {
      next.delete(k);
      if (cutoffKey === k) setCutoffKey("");
    } else {
      next.add(k);
      if (!cutoffKey) setCutoffKey(k);
    }
    setSelectedKeys(next);
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) {
      toast.error("Name is required");
      return;
    }
    if (selectedQualities.length === 0) {
      toast.error("Select at least one quality");
      return;
    }
    if (!cutoffQuality) {
      toast.error("Select a cutoff quality");
      return;
    }

    const body = {
      name: name.trim(),
      cutoff_json: JSON.stringify(cutoffQuality),
      qualities_json: JSON.stringify(selectedQualities),
      upgrade_allowed: upgradeAllowed,
    };

    const onSuccess = () => onClose();

    if (isEdit) {
      update.mutate({ id: initial!.id, ...body }, { onSuccess });
    } else {
      create.mutate(body, { onSuccess });
    }
  }

  return (
    <Modal onClose={onClose} width={560}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          padding: "18px 20px",
          borderBottom: "1px solid var(--color-border-subtle)",
        }}
      >
        <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>
          {isEdit ? "Edit Quality Profile" : "New Quality Profile"}
        </h2>
        <button
          onClick={onClose}
          style={{ background: "none", border: "none", cursor: "pointer", color: "var(--color-text-muted)", display: "flex", padding: 4 }}
        >
          <X size={18} />
        </button>
      </div>

      <form
        onSubmit={handleSubmit}
        style={{ padding: 20, display: "flex", flexDirection: "column", gap: 16, maxHeight: "70vh", overflowY: "auto" }}
      >
        <div>
          <label style={labelStyle}>Name</label>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            placeholder="e.g. HD-1080p"
            style={inputStyle}
          />
        </div>

        <div>
          <label style={labelStyle}>Allowed Qualities</label>
          <div style={{ fontSize: 11, color: "var(--color-text-muted)", marginBottom: 8 }}>
            Select one or more quality tiers. Higher tiers are preferred over lower ones.
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
            {QUALITY_TIERS.slice().reverse().map((q) => {
              const k = qualityKey(q);
              const isSelected = selectedKeys.has(k);
              return (
                <label
                  key={k}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 10,
                    padding: "6px 10px",
                    borderRadius: 4,
                    cursor: "pointer",
                    background: isSelected ? "var(--color-bg-elevated)" : "transparent",
                    border: `1px solid ${isSelected ? "var(--color-border-default)" : "transparent"}`,
                  }}
                >
                  <input
                    type="checkbox"
                    checked={isSelected}
                    onChange={() => toggleQuality(q)}
                    style={{ accentColor: "var(--color-accent)" }}
                  />
                  <span style={{ fontSize: 13, color: "var(--color-text-primary)" }}>{q.name}</span>
                </label>
              );
            })}
          </div>
        </div>

        <div>
          <label style={labelStyle}>Cutoff</label>
          <div style={{ fontSize: 11, color: "var(--color-text-muted)", marginBottom: 8 }}>
            Quality that's considered "good enough". No further upgrades once this is reached.
          </div>
          <select
            value={cutoffKey}
            onChange={(e) => setCutoffKey(e.target.value)}
            disabled={selectedQualities.length === 0}
            style={inputStyle}
          >
            <option value="">— Select cutoff —</option>
            {selectedQualities.map((q) => {
              const k = qualityKey(q);
              return (
                <option key={k} value={k}>
                  {q.name}
                </option>
              );
            })}
          </select>
        </div>

        <div>
          <label
            style={{
              display: "flex",
              alignItems: "center",
              gap: 8,
              cursor: "pointer",
              color: "var(--color-text-secondary)",
              fontSize: 13,
            }}
          >
            <input
              type="checkbox"
              checked={upgradeAllowed}
              onChange={(e) => setUpgradeAllowed(e.target.checked)}
              style={{ accentColor: "var(--color-accent)" }}
            />
            Allow upgrades (replace lower-quality files with higher-quality ones)
          </label>
        </div>

        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8, paddingTop: 4 }}>
          <button type="button" onClick={onClose} style={cancelButtonStyle}>
            Cancel
          </button>
          <button type="submit" disabled={isPending} style={{ ...submitButtonStyle, opacity: isPending ? 0.7 : 1 }}>
            {isPending ? "Saving..." : isEdit ? "Save Changes" : "Create Profile"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

export default function QualityProfilesPage() {
  const { data: profiles, isLoading } = useQualityProfiles();
  const deleteMut = useDeleteQualityProfile();
  const confirm = useConfirm();
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<QualityProfile | null>(null);

  async function handleDelete(p: QualityProfile) {
    if (
      !(await confirm({
        title: "Delete quality profile",
        message: `Delete "${p.name}"? Services using it will lose their sync and fall back to defaults.`,
        confirmLabel: "Delete",
      }))
    )
      return;
    deleteMut.mutate(p.id);
  }

  return (
    <div style={{ padding: 24, maxWidth: 900 }}>
      <PageHeader
        title="Quality Profiles"
        description="Centrally defined quality profiles pushed to media-manager services (Prism, Pilot) via their sync loops."
        action={
          <button onClick={() => setShowForm(true)} style={addButtonStyle}>
            <Plus size={15} strokeWidth={2.5} />
            Add Profile
          </button>
        }
      />

      {isLoading && (
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          {Array.from({ length: 2 }).map((_, i) => (
            <div key={i} className="skeleton" style={{ height: 72, borderRadius: 8 }} />
          ))}
        </div>
      )}

      {!isLoading && (!profiles || profiles.length === 0) && (
        <div
          style={{
            textAlign: "center",
            padding: "48px 24px",
            background: "var(--color-bg-surface)",
            border: "1px solid var(--color-border-subtle)",
            borderRadius: 8,
            color: "var(--color-text-muted)",
            fontSize: 14,
          }}
        >
          No quality profiles defined yet. Create one and it will be synced to all media-manager services.
        </div>
      )}

      {!isLoading && profiles && profiles.length > 0 && (
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          {profiles.map((p) => {
            let qualities: Quality[] = [];
            let cutoff: Quality | null = null;
            try {
              qualities = JSON.parse(p.qualities_json);
              cutoff = p.cutoff_json && p.cutoff_json !== "{}" ? JSON.parse(p.cutoff_json) : null;
            } catch {
              // ignore parse errors
            }

            return (
              <div key={p.id} style={{ ...card, padding: "14px 16px", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                  <div style={{ fontSize: 14, fontWeight: 600, color: "var(--color-text-primary)" }}>
                    {p.name}
                  </div>
                  <div style={{ fontSize: 12, color: "var(--color-text-muted)" }}>
                    {qualities.length} {qualities.length === 1 ? "quality" : "qualities"}
                    {cutoff && ` · cutoff: ${cutoff.name}`}
                    {p.upgrade_allowed && " · upgrades allowed"}
                  </div>
                </div>
                <div style={{ display: "flex", gap: 8 }}>
                  <button onClick={() => setEditing(p)} style={iconButtonStyle} title="Edit">
                    <Pencil size={14} strokeWidth={1.5} />
                  </button>
                  <button
                    onClick={() => handleDelete(p)}
                    style={{ ...iconButtonStyle, color: "var(--color-danger)" }}
                    title="Delete"
                  >
                    <Trash2 size={14} strokeWidth={1.5} />
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {showForm && <ProfileForm onClose={() => setShowForm(false)} />}
      {editing && <ProfileForm initial={editing} onClose={() => setEditing(null)} />}
    </div>
  );
}

const labelStyle: React.CSSProperties = {
  display: "block",
  fontSize: 12,
  fontWeight: 500,
  color: "var(--color-text-secondary)",
  marginBottom: 6,
};

const inputStyle: React.CSSProperties = {
  width: "100%",
  padding: "8px 12px",
  background: "var(--color-bg-elevated)",
  border: "1px solid var(--color-border-default)",
  borderRadius: 6,
  fontSize: 14,
  color: "var(--color-text-primary)",
  outline: "none",
};

const cancelButtonStyle: React.CSSProperties = {
  padding: "8px 16px",
  background: "none",
  border: "1px solid var(--color-border-default)",
  borderRadius: 6,
  cursor: "pointer",
  fontSize: 13,
  color: "var(--color-text-secondary)",
};

const submitButtonStyle: React.CSSProperties = {
  padding: "8px 16px",
  background: "var(--color-accent)",
  border: "none",
  borderRadius: 6,
  cursor: "pointer",
  fontSize: 13,
  fontWeight: 600,
  color: "#fff",
};

const iconButtonStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  padding: "6px 10px",
  background: "none",
  border: "1px solid var(--color-border-default)",
  borderRadius: 6,
  cursor: "pointer",
  color: "var(--color-text-secondary)",
};

const addButtonStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 6,
  padding: "8px 14px",
  background: "var(--color-accent)",
  border: "none",
  borderRadius: 6,
  cursor: "pointer",
  fontSize: 13,
  fontWeight: 600,
  color: "#fff",
};
