export function registrationTokenFromLocation(): string {
  const params = new URLSearchParams(window.location.hash.slice(1));
  if (!params.has("register")) return "";
  const token = params.get("register") ?? "";
  return /^[A-Za-z0-9_-]{43}$/.test(token) ? token : "invalid";
}

export function clearRegistrationFragment(): void {
  if (new URLSearchParams(window.location.hash.slice(1)).has("register")) {
    window.history.replaceState(window.history.state, "", window.location.pathname + window.location.search);
  }
}
