import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
      "@beacon-shared": path.resolve(__dirname, "../../../web-shared"),
      // Force React singleton. See pilot/web/ui/vite.config.ts for context.
      react: path.resolve(__dirname, "node_modules/react"),
      "react-dom": path.resolve(__dirname, "node_modules/react-dom"),
    },
    // react-router and react-router-dom must be singletons so the Router
    // context provided by BrowserRouter is the same instance the shared
    // web-shared/Shell.tsx reads via useLocation/Link/NavLink. Without
    // dedupe, Shell picks up a separate copy from web-shared/node_modules
    // and throws "useLocation may be used only in the context of a
    // <Router>". Using dedupe (not aliases) lets Vite resolve subpath
    // exports like `react-router/dom` correctly.
    dedupe: ["react", "react-dom", "react-router-dom", "react-router"],
  },
  build: {
    outDir: "../static",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:9696",
        changeOrigin: true,
        ws: true,
      },
    },
  },
});
