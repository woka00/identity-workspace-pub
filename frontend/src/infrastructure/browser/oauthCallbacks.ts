type FatSecretCallbackStatus = "connected" | "denied" | "expired" | "error";

export interface IntegrationNotice {
  tone: "success" | "error";
  text: string;
}

function consumeCallbackStatus(parameter: string) {
  if (typeof window === "undefined") return null;
  const url = new URL(window.location.href);
  const status = url.searchParams.get(parameter);
  if (!status) return null;

  url.searchParams.delete(parameter);
  window.history.replaceState({}, "", `${url.pathname}${url.search}${url.hash}`);
  return status;
}

export function consumeFatSecretCallbackNotice(): IntegrationNotice | null {
  const status = consumeCallbackStatus("fatsecret") as FatSecretCallbackStatus | null;
  if (!status) return null;
  if (status === "connected") {
    return { tone: "success", text: "Существующий аккаунт FatSecret успешно подключён." };
  }
  if (status === "denied") {
    return { tone: "error", text: "Вы не подтвердили доступ к аккаунту FatSecret." };
  }
  if (status === "expired") {
    return { tone: "error", text: "Срок попытки подключения истёк. Запустите вход ещё раз." };
  }
  return { tone: "error", text: "Не удалось подключить аккаунт FatSecret. Повторите попытку." };
}
