import { useEffect, useRef, useState } from "react";

export default function TaskScheduleInput({ type, value, disabled = false, onChange }: {
  type: "time" | "datetime-local";
  value: string;
  disabled?: boolean;
  onChange: (value: string) => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [version, setVersion] = useState(0);

  useEffect(() => {
    const input = inputRef.current;
    if (input && input.value !== value) input.value = value;
  }, [value, version]);

  useEffect(() => {
    const input = inputRef.current;
    if (!input) return;
    const clear = () => {
      input.value = "";
      onChange("");
      // Safari may retain date/time segments and badInput after its picker Reset.
      setVersion((current) => current + 1);
    };
    const sync = () => {
      if (!input.value && !input.validity.badInput) clear();
      else onChange(input.value);
    };
    // Native change is a committed picker selection/reset, unlike partial keyboard input.
    const commit = () => { if (!input.value) clear(); else onChange(input.value); };
    input.addEventListener("input", sync);
    input.addEventListener("change", commit);
    input.addEventListener("blur", commit);
    return () => {
      input.removeEventListener("input", sync);
      input.removeEventListener("change", commit);
      input.removeEventListener("blur", commit);
    };
  }, [onChange, version]);

  return <input key={version} ref={inputRef} className="input" type={type} defaultValue="" disabled={disabled} aria-label={type === "time" ? "Время задачи" : "Напомнить"} />;
}
