import { useState } from "react";
import IntroductionDialog from "../onboarding/IntroductionDialog";
import { FeatureIcon } from "../onboarding/WorkspaceOnboarding";

export default function TimerIntroduction({ onDone }: { onDone: () => Promise<void> }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function finish() {
    if (busy) return;
    setBusy(true);
    setError("");
    try { await onDone(); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Не удалось сохранить. Попробуйте ещё раз."); }
    finally { setBusy(false); }
  }
  return <IntroductionDialog titleID="timer-intro-title" onClose={busy ? undefined : () => void finish()}>
    <header className="introductionHeading">
      <span className="introductionKicker">Знакомство с таймером</span>
      <h1 id="timer-intro-title">Ваше время.<br />В поле зрения.</h1>
      <p>Секундомер для того, чем вы занимаетесь. Помогает увидеть, сколько времени уходит на работу, учёбу и отдых.</p>
    </header>
    <div className="timerIntroPreview" aria-label="Пример таймера: Чтение, 25 минут">
      <span className="timerIntroActivity"><FeatureIcon feature="tracker" />Чтение<span>Пример</span></span>
      <div className="timerIntroClock" aria-hidden="true">00<span>:</span>25<span>:</span>00</div>
      <div className="timerIntroCaption"><span>часы</span><span>минуты</span><span>секунды</span></div>
    </div>
    <ol className="timerIntroSteps">
      <li><span>01</span><div><strong>Добавьте занятие</strong><p>Нажмите «＋ Задача» и назовите активность: например, «Чтение» или «Разработка».</p></div></li>
      <li><span>02</span><div><strong>Запустите и занимайтесь своим делом</strong><p>Нажмите ▶ рядом с активностью. «Пауза» остановит отсчёт, «Завершить» закончит занятие. Время сохранится в обоих случаях.</p></div></li>
      <li><span>03</span><div><strong>Посмотрите, как прошёл день</strong><p>Откройте блок «Сегодня», чтобы увидеть подробную статистику и распределение времени.</p></div></li>
    </ol>
    <footer className="introductionFooter">
      <p>Одновременно работает один таймер. Он продолжает считать, даже если вы закроете приложение.</p>
      {error && <p className="introductionError" role="alert">{error}</p>}
      <button type="button" className="primaryButton introductionSubmit" disabled={busy} onClick={() => void finish()}>{busy ? "Сохраняем…" : "Понятно, начнём"}<span aria-hidden="true">→</span></button>
    </footer>
  </IntroductionDialog>;
}
