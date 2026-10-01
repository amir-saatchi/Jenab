import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

// Placeholder until the app shell (P1-14) brings over mockups/src.
function App() {
  return (
    <main style={{ fontFamily: "system-ui, sans-serif", padding: 32 }}>
      <h1>Jenab</h1>
      <p>The app shell comes with P1-14.</p>
    </main>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
