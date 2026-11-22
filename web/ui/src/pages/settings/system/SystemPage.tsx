import PageHeader from "@/components/PageHeader";
import SystemInfo from "@beacon-shared/SystemInfo";

export default function SystemPage() {
  return (
    <div style={{ padding: 24, maxWidth: 900 }}>
      <PageHeader title="System" description="What's running and how it's doing." />
      <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
        <SystemInfo app="Pulse" service="pulse" image="ghcr.io/beacon-stack/pulse" />
      </div>
    </div>
  );
}
