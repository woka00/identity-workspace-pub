import { useRef, useState, type ChangeEvent, type FormEvent } from "react";
import type { Profile } from "../../domain/models";
import { api } from "../../infrastructure/http/apiClient";

type Props = {
  login: string;
  profile: Profile;
  onClose: () => void;
  onSaved: () => Promise<void>;
};

function initials(value: string) {
  const parts = value.trim().split(/\s+/).filter(Boolean);
  return (parts.length > 1 ? parts.slice(0, 2).map((part) => part[0]).join("") : parts[0]?.slice(0, 2) || "У").toUpperCase();
}

async function avatarData(file: File) {
  const url = URL.createObjectURL(file);
  try {
    const image = await new Promise<HTMLImageElement>((resolve, reject) => {
      const element = new Image();
      element.onload = () => resolve(element);
      element.onerror = () => reject(new Error("Не удалось прочитать изображение"));
      element.src = url;
    });
    const size = 512;
    const canvas = document.createElement("canvas");
    canvas.width = size;
    canvas.height = size;
    const context = canvas.getContext("2d");
    if (!context) throw new Error("Не удалось обработать изображение");
    const sourceSize = Math.min(image.naturalWidth, image.naturalHeight);
    const sourceX = (image.naturalWidth - sourceSize) / 2;
    const sourceY = (image.naturalHeight - sourceSize) / 2;
    context.imageSmoothingEnabled = true;
    context.imageSmoothingQuality = "high";
    context.drawImage(image, sourceX, sourceY, sourceSize, sourceSize, 0, 0, size, size);
    return canvas.toDataURL("image/webp", .9);
  } finally {
    URL.revokeObjectURL(url);
  }
}

export default function ProjectProfileDialog({ login, profile, onClose, onSaved }: Props) {
  const [displayName, setDisplayName] = useState(profile.workDisplayName || login);
  const [avatar, setAvatar] = useState(profile.workAvatar);
  const [showAvatar, setShowAvatar] = useState(profile.workShowAvatar);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const fileRef = useRef<HTMLInputElement>(null);
  const previewName = displayName.trim() || login;
  const previewAvatar = showAvatar ? avatar || profile.photo : "";

  async function pickAvatar(event: ChangeEvent<HTMLInputElement>) {
    const file = event.currentTarget.files?.[0];
    event.currentTarget.value = "";
    if (!file) return;
    setError("");
    try {
      setAvatar(await avatarData(file));
      setShowAvatar(true);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await api.updateWorkProfile({ displayName: displayName.trim(), avatar, showAvatar });
      await onSaved();
      onClose();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(false);
    }
  }

  return <div className="projectProfileBackdrop" role="presentation" onMouseDown={(event)=>{if(event.target===event.currentTarget&&!busy)onClose()}}>
    <form className="projectProfileDialog" noValidate onSubmit={submit} role="dialog" aria-modal="true" aria-labelledby="project-profile-title">
      <header><div><span>Профиль в проектах</span><h2 id="project-profile-title">Как вас видят участники</h2></div><button type="button" onClick={onClose} disabled={busy} aria-label="Закрыть">×</button></header>
      <div className="projectProfilePreview">
        <span className="projectProfileAvatar">{previewAvatar ? <img src={previewAvatar} alt=""/> : initials(previewName)}</span>
        <div><strong>{previewName}</strong><small>Так профиль выглядит для других участников</small></div>
      </div>
      <label className="projectProfileField">Ник<input value={displayName} maxLength={80} onChange={(event)=>setDisplayName(event.target.value)} placeholder={login}/></label>
      <div className="projectProfilePhotoActions">
        <button type="button" onClick={()=>fileRef.current?.click()}>Загрузить фото</button>
        <button type="button" onClick={()=>{setAvatar("");setShowAvatar(true)}} disabled={!avatar}>Использовать фото карточки</button>
        <input ref={fileRef} hidden type="file" accept="image/png,image/jpeg,image/webp" onChange={(event)=>void pickAvatar(event)}/>
      </div>
      <label className="projectProfileToggle"><span><strong>Показывать фото</strong><small>{avatar ? "Используется отдельное фото профиля" : profile.photo ? "Используется фото из карточки" : "Если фото нет, показываются инициалы"}</small></span><input type="checkbox" checked={showAvatar} onChange={(event)=>setShowAvatar(event.target.checked)}/><i aria-hidden="true"/></label>
      {error && <div className="projectProfileError">{error}</div>}
      <div className="projectProfileActions"><button type="button" onClick={onClose} disabled={busy}>Отмена</button><button type="submit" disabled={busy}>{busy?"Сохраняем…":"Сохранить"}</button></div>
    </form>
  </div>;
}
