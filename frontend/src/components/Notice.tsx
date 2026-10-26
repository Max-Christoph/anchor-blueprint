export type NoticeTone = "ok" | "error";

export interface NoticeState {
  tone: NoticeTone;
  text: string;
}

const TONE_CLASSES: Record<NoticeTone, string> = {
  ok: "border-emerald-200 bg-emerald-50 text-emerald-800",
  error: "border-rose-200 bg-rose-50 text-rose-800",
};

interface NoticeProps {
  notice: NoticeState;
  onDismiss: () => void;
}

export function Notice({ notice, onDismiss }: NoticeProps) {
  return (
    <div
      role="status"
      className={`mb-6 flex items-start justify-between gap-4 rounded-xl border px-4 py-3 text-sm ${TONE_CLASSES[notice.tone]}`}
    >
      <span>{notice.text}</span>
      <button type="button" onClick={onDismiss} className="text-xs font-medium underline">
        schließen
      </button>
    </div>
  );
}
