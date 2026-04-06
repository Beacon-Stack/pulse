import { useEffect } from "react";
import type { ReactNode, CSSProperties } from "react";

interface ModalProps {
  onClose: () => void;
  children: ReactNode;
  width?: number | string;
  maxWidth?: string;
  maxHeight?: string;
  innerStyle?: CSSProperties;
}

export default function Modal({
  onClose,
  children,
  width = 520,
  maxWidth = "calc(100vw - 48px)",
  maxHeight = "calc(100vh - 80px)",
  innerStyle,
}: ModalProps) {
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,0.6)",
        backdropFilter: "blur(2px)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        zIndex: 200,
      }}
      onClick={onClose}
    >
      <div
        style={{
          background: "var(--color-bg-surface)",
          border: "1px solid var(--color-border-subtle)",
          borderRadius: 12,
          width,
          maxWidth,
          maxHeight,
          boxShadow: "var(--shadow-modal)",
          display: "flex",
          flexDirection: "column",
          overflow: "hidden",
          ...innerStyle,
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {children}
      </div>
    </div>
  );
}
