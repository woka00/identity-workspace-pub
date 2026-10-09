import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import type { AuthUser } from "../../domain/models";
import { api } from "../../infrastructure/http/apiClient";

type Mode = "login" | "request" | "complete";

export function AuthScreen({ serverError, onAuthenticated, onRetry, registrationToken = "", onDiscardToken }: {
  serverError: string;
  onAuthenticated: (user: AuthUser) => void;
  onRetry: () => Promise<void>;
  registrationToken?: string;
  onDiscardToken: () => void;
}) {
  const [mode, setMode] = useState<Mode>(registrationToken ? "complete" : "login");
  const [registrationEnabled, setRegistrationEnabled] = useState(false);
  const [login, setLogin] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [email, setEmail] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [resendAt, setResendAt] = useState(0);
  const [secondsLeft, setSecondsLeft] = useState(0);

  useEffect(() => {
    let active = true;
    void api.registrationConfig().then((config) => { if (active) setRegistrationEnabled(config.enabled); }).catch(() => {});
    return () => { active = false; };
  }, []);

  useEffect(() => {
    if (registrationToken) { setMode("complete"); setPassword(""); setConfirmation(""); setError(""); setNotice(""); }
  }, [registrationToken]);

  useEffect(() => {
    const update = () => setSecondsLeft(Math.max(0, Math.ceil((resendAt - Date.now()) / 1000)));
    update();
    if (!resendAt) return;
    const timer = window.setInterval(update, 1000);
    return () => window.clearInterval(timer);
  }, [resendAt]);

  function switchMode(next: Mode) {
    setMode(next); setError(""); setNotice(""); setPassword(""); setConfirmation("");
    if (next !== "complete") onDiscardToken();
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (submitting) return;
    setError(""); setNotice("");
    if (mode === "complete" && password !== confirmation) { setError("Пароли не совпадают"); return; }
    setSubmitting(true);
    try {
      if (mode === "login") {
        const response = await api.login(login, password);
        onAuthenticated(response.user);
      } else if (mode === "request") {
        const response = await api.requestRegistration(email);
        setNotice(response.message);
        setResendAt(Date.now() + 60_000);
      } else {
        await api.completeRegistration(registrationToken, login, password);
        setPassword(""); setConfirmation(""); onDiscardToken(); setMode("login");
        setNotice("Почта подтверждена, аккаунт создан. Войдите с выбранными логином и паролем.");
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось выполнить запрос");
    } finally { setSubmitting(false); }
  }

  const title = mode === "login" ? "Вход" : "Регистрация";
  return <main className="authPage">
    <section className="authDocument" aria-labelledby="auth-title">
      <header className="authDocumentHead"><span>identity workspace</span><span>YOUR WORKSPACE</span></header>
      <div className="authDocumentBody">
        <div className="authSerial">{mode === "login" ? "FORM · A-01" : "FORM · A-02"}</div>
        <h1 id="auth-title">{title}</h1>
        <p>{mode === "login" ? "Введите логин и пароль вашего аккаунта." : mode === "request" ? "Шаг 1 из 2. Отправим на вашу почту ссылку. После подтверждения вы выберете логин и пароль." : "Шаг 2 из 2. Выберите логин и пароль, чтобы подтвердить почту и создать аккаунт."}</p>
        <form className="authForm" noValidate onSubmit={submit}>
          {mode === "request" ? <label className="field"><span className="fieldLabel">Почта</span>
            <input className="input authInput" type="email" autoFocus value={email} onChange={event => setEmail(event.target.value)} autoComplete="email" autoCapitalize="none" spellCheck={false} maxLength={254} required disabled={submitting} />
          </label> : <>
            <label className="field"><span className="fieldLabel">Логин</span>
              <input className="input authInput" autoFocus value={login} onChange={event => setLogin(event.target.value)} autoComplete="username" autoCapitalize="none" spellCheck={false} minLength={3} maxLength={32} required disabled={submitting} />
            </label>
            {mode === "complete" && <p className="authHelp">От 3 до 32 символов: буквы, цифры, точка, дефис или подчёркивание.</p>}
            <label className="field"><span className="fieldLabel">Пароль</span>
              <input className="input authInput" type="password" value={password} onChange={event => setPassword(event.target.value)} autoComplete={mode === "login" ? "current-password" : "new-password"} minLength={mode === "login" ? 8 : 15} maxLength={128} required disabled={submitting} />
            </label>
            {mode === "complete" && <>
              <p className="authHelp">От 15 до 128 символов. Можно использовать длинную парольную фразу.</p>
              <label className="field"><span className="fieldLabel">Повторите пароль</span>
                <input className="input authInput" type="password" value={confirmation} onChange={event => setConfirmation(event.target.value)} autoComplete="new-password" minLength={15} maxLength={128} required disabled={submitting} />
              </label>
            </>}
          </>}
          {(error || serverError) && <div className="formError" role="alert">{error || serverError}</div>}
          {notice && <p className="authNotice" role="status">{notice}</p>}
          {serverError && <button type="button" className="textButton authRetry" onClick={() => void onRetry()}>Проверить сервер ещё раз</button>}
          <button className="primaryButton authSubmit" disabled={submitting || (mode === "request" && secondsLeft > 0)}>
            {submitting ? "ПОДОЖДИТЕ…" : mode === "login" ? "ВОЙТИ" : mode === "complete" ? "ПОДТВЕРДИТЬ И СОЗДАТЬ АККАУНТ" : secondsLeft > 0 ? `ПОВТОРИТЬ ЧЕРЕЗ ${secondsLeft} С` : "ОТПРАВИТЬ ССЫЛКУ"}
          </button>
        </form>
        <div className="authActions">
          {mode === "login" && registrationEnabled && <button className="textButton" disabled={submitting} onClick={() => switchMode("request")}>Создать аккаунт</button>}
          {mode !== "login" && <button className="textButton" disabled={submitting} onClick={() => switchMode("login")}>Уже есть аккаунт? Войти</button>}
          {mode === "complete" && registrationEnabled && <button className="textButton" disabled={submitting} onClick={() => switchMode("request")}>Запросить новую ссылку</button>}
        </div>
      </div>
      <footer className="authDocumentFoot"><span>identity workspace</span><span>PERSONAL SPACE</span></footer>
    </section>
  </main>;
}
