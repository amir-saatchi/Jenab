import { StrictMode, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { ProjectService } from "../bindings/github.com/amir-saatchi/jenab/internal/app";

// Placeholder until the app shell (P1-14) brings over mockups/src. The
// project count shows the Go services are reachable.
function App() {
  const [status, setStatus] = useState("Connecting…");
  useEffect(() => {
    ProjectService.List()
      .then((ps) => setStatus(`${(ps ?? []).length} project(s)`))
      .catch((err: unknown) => setStatus(`Error: ${String(err)}`));
  }, []);
  return (
    <main style={{ fontFamily: "system-ui, sans-serif", padding: 32 }}>
      <h1>Jenab</h1>
      <p>The app shell comes with P1-14.</p>
      <p>{status}</p>
    </main>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
