import { useEffect } from "react";
import type { ReactNode, CSSProperties } from "react";

interface DrawerProps {
  onClose: () => void;
  children: ReactNode;
  /** Width of the inner panel. Default: 600. */
  width?: number | string;
  /** Side to dock to. Default: right. */
  side?: "left" | "right";
  /** Extra styles merged onto the inner panel. */
  innerStyle?: CSSProperties;
}

/**
 * Side-docked drawer for drill-down views (Linear / Sentry style). Behavior
 * matches Modal: escape key closes, click on the backdrop closes, click on
 * the drawer itself does not. Slides in from the chosen side; on mobile
 * (≤640px effective viewport) the panel goes full-width.
 *
 * Theme variables match Modal so a service swapping between drawer and
 * modal sees identical chrome.
 */
export default function Drawer({
  onClose,
  children,
  width = 600,
  side = "right",
  innerStyle,
}: DrawerProps) {
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  const dockStyles: CSSProperties =
    side === "right"
      ? { right: 0, borderLeft: "1px solid var(--color-border-subtle)" }
      : { left: 0, borderRight: "1px solid var(--color-border-subtle)" };

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,0.6)",
        backdropFilter: "blur(2px)",
        zIndex: 200,
      }}
      onClick={onClose}
    >
      <div
        style={{
          position: "absolute",
          top: 0,
          height: "100vh",
          width,
          maxWidth: "100vw",
          background: "var(--color-bg-surface)",
          boxShadow: "var(--shadow-modal)",
          display: "flex",
          flexDirection: "column",
          overflow: "hidden",
          ...dockStyles,
          ...innerStyle,
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {children}
      </div>
    </div>
  );
}
