import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./styles.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);

if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    void navigator.serviceWorker.register("/sw.js?v=identity-workspace-icon-v2", { scope: "/", updateViaCache: "none" }).catch(() => {
      // Напоминания останутся недоступны, приложение продолжит работать.
    });
  });
}
