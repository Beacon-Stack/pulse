import { useState } from "react";
import { card } from "@/lib/styles";

export interface ActiveColumn<T> {
  /** Column header. */
  header: string;
  /** Renders the cell value for this row. */
  cell: (row: T) => React.ReactNode;
  /** Optional column width in px. */
  width?: number;
  /** Right-align the cell (good for numerics). */
  numeric?: boolean;
}

interface ActivePanelProps<T> {
  title: string;
  subtitle?: string;
  rows: T[];
  total: number;
  columns: ActiveColumn<T>[];
  /** Stable row key — used by React and the expand/collapse mechanism. */
  rowKey: (row: T) => string;
  /** Empty-state message. If items.length === 0 AND total === 0, panel is hidden. */
  emptyText?: string;
}

/**
 * Generic "what's happening right now" table — shared by the dashboard's
 * Active downloads and Active imports panels. Renders the first `rows`
 * inline; if `total` exceeds `rows.length`, shows a "+ N more" toggle
 * that expands an inline overflow list.
 *
 * Designed to be polymorphic on the row shape so adding a new "active
 * X" feed in the future just means defining columns + rows + a key.
 */
export default function ActivePanel<T>({
  title,
  subtitle,
  rows,
  total,
  columns,
  rowKey,
  emptyText,
}: ActivePanelProps<T>) {
  const [expanded, setExpanded] = useState(false);

  if (total === 0 && rows.length === 0) return null;

  const overflow = Math.max(0, total - rows.length);

  return (
    <div style={{ ...card, padding: 16 }}>
      <div
        style={{
          display: "flex",
          alignItems: "baseline",
          justifyContent: "space-between",
          marginBottom: 10,
        }}
      >
        <div>
          <div
            style={{
              fontSize: 13,
              fontWeight: 600,
              color: "var(--color-text-primary)",
            }}
          >
            {title}
          </div>
          {subtitle && (
            <div style={{ fontSize: 11, color: "var(--color-text-muted)", marginTop: 2 }}>
              {subtitle}
            </div>
          )}
        </div>
        <div style={{ fontSize: 11, color: "var(--color-text-muted)", fontVariantNumeric: "tabular-nums" }}>
          {total === 1 ? "1 item" : `${total} items`}
        </div>
      </div>

      {rows.length === 0 ? (
        <div style={{ fontSize: 12, color: "var(--color-text-muted)", padding: "8px 0" }}>
          {emptyText ?? "Nothing right now."}
        </div>
      ) : (
        <ActiveTable rows={rows} columns={columns} rowKey={rowKey} />
      )}

      {overflow > 0 && (
        <div style={{ marginTop: 8 }}>
          <button
            onClick={() => setExpanded((v) => !v)}
            style={{
              background: "transparent",
              border: "none",
              padding: "4px 0",
              fontSize: 11,
              color: "var(--color-text-secondary)",
              cursor: "pointer",
              fontWeight: 500,
            }}
          >
            {expanded ? "show less" : `+ ${overflow} more`}
          </button>
        </div>
      )}
    </div>
  );
}

function ActiveTable<T>({
  rows,
  columns,
  rowKey,
}: {
  rows: T[];
  columns: ActiveColumn<T>[];
  rowKey: (row: T) => string;
}) {
  return (
    <div style={{ overflowX: "auto" }}>
      <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 12 }}>
        <thead>
          <tr style={{ borderBottom: "1px solid var(--color-border-subtle)" }}>
            {columns.map((col, i) => (
              <th
                key={i}
                style={{
                  textAlign: col.numeric ? "right" : "left",
                  padding: "6px 8px",
                  fontSize: 10,
                  fontWeight: 600,
                  color: "var(--color-text-muted)",
                  textTransform: "uppercase",
                  letterSpacing: "0.06em",
                  width: col.width,
                  whiteSpace: "nowrap",
                }}
              >
                {col.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={rowKey(row)} style={{ borderBottom: "1px solid var(--color-border-subtle)" }}>
              {columns.map((col, i) => (
                <td
                  key={i}
                  style={{
                    padding: "6px 8px",
                    color: "var(--color-text-primary)",
                    fontVariantNumeric: col.numeric ? "tabular-nums" : undefined,
                    textAlign: col.numeric ? "right" : "left",
                    maxWidth: col.width,
                    whiteSpace: "nowrap",
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                  }}
                >
                  {col.cell(row)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
