import type { ReactNode } from "react";
import { BrowserRouter, Routes, Route, useLocation } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Toaster } from "sonner";
import { ConfirmProvider } from "@beacon-shared/ConfirmDialog";
import { Shell } from "@/layouts/Shell";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import Dashboard from "@/pages/dashboard/Dashboard";
import ServicesPage from "@/pages/services/ServicesPage";
import ServiceDetailPage from "@/pages/services/ServiceDetailPage";
import IndexersPage from "@/pages/indexers/IndexersPage";
import AddIndexerPage from "@/pages/indexers/AddIndexerPage";
import IndexerDetailPage from "@/pages/indexers/IndexerDetailPage";
import DownloadClientsPage from "@/pages/download-clients/DownloadClientsPage";
import DownloadClientDetailPage from "@/pages/download-clients/DownloadClientDetailPage";
import QualityProfilesPage from "@/pages/quality-profiles/QualityProfilesPage";
import SharedSettingsPage from "@/pages/shared-settings/SharedSettingsPage";
import SystemPage from "@/pages/settings/system/SystemPage";
import AppSettingsPage from "@/pages/settings/app/AppSettingsPage";

function RouteEB({ children }: { children: ReactNode }) {
  const { pathname } = useLocation();
  return <ErrorBoundary resetKey={pathname}>{children}</ErrorBoundary>;
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 30_000,
    },
  },
});

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ConfirmProvider>
        <BrowserRouter>
          <ErrorBoundary>
            <Routes>
              <Route element={<Shell />}>
                <Route index element={<RouteEB><Dashboard /></RouteEB>} />
                <Route path="services" element={<RouteEB><ServicesPage /></RouteEB>} />
                <Route path="services/:id" element={<RouteEB><ServiceDetailPage /></RouteEB>} />
                <Route path="indexers" element={<RouteEB><IndexersPage /></RouteEB>} />
                <Route path="indexers/add" element={<RouteEB><AddIndexerPage /></RouteEB>} />
                <Route path="indexers/:id" element={<RouteEB><IndexerDetailPage /></RouteEB>} />
                <Route path="download-clients" element={<RouteEB><DownloadClientsPage /></RouteEB>} />
                <Route path="download-clients/:id" element={<RouteEB><DownloadClientDetailPage /></RouteEB>} />
                <Route path="quality-profiles" element={<RouteEB><QualityProfilesPage /></RouteEB>} />
                <Route path="shared-settings" element={<RouteEB><SharedSettingsPage /></RouteEB>} />
                <Route path="settings">
                  <Route path="system" element={<RouteEB><SystemPage /></RouteEB>} />
                  <Route path="app" element={<RouteEB><AppSettingsPage /></RouteEB>} />
                </Route>
              </Route>
            </Routes>
          </ErrorBoundary>
          <Toaster
            position="bottom-right"
            toastOptions={{
              style: {
                background: "var(--color-bg-elevated)",
                border: "1px solid var(--color-border-default)",
                color: "var(--color-text-primary)",
                fontSize: 13,
              },
            }}
          />
        </BrowserRouter>
      </ConfirmProvider>
    </QueryClientProvider>
  );
}
