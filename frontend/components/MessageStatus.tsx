"use client";

export type MessageStatusType = "sent" | "delivered" | "seen";

export function MessageStatusIcon({
  status,
  size = 14,
}: {
  status: MessageStatusType;
  size?: number;
}) {
  if (status === "seen") {
    return (
      <svg
        width={size}
        height={size * 0.83}
        viewBox="0 0 36 30"
        fill="rgba(74,222,128,0.9)"
        aria-label="Görüldü"
        role="img"
      >
        <path d="M6,28 C5,26 4,22 5,17 C6,13 8,9 7,5 C6.5,3 5,2 6,1 C7,0.5 8,1 8,3 C9,7 7,12 8,16 C9,20 11,24 10,28 Z" />
        <path d="M16,29 C15,27 14,23 15,17 C16,12 18,7 17,3 C16.5,1 15,0 16,0 C17,0 18,1 18,3 C19,8 17,13 18,18 C19,22 21,26 20,29 Z" />
        <path d="M28,28 C29,26 30,22 29,17 C28,13 26,9 27,5 C27.5,3 29,2 28,1 C27,0.5 26,1 26,3 C25,7 27,12 26,16 C25,20 23,24 24,28 Z" />
        <path d="M8,4 C10,1 15,0 18,0 C21,0 26,2 28,4 C25,2 20,1 18,1 C16,1 11,2 8,4 Z" />
      </svg>
    );
  }

  if (status === "delivered") {
    return (
      <svg
        width={size}
        height={size * 0.6}
        viewBox="0 0 30 18"
        fill="rgba(232,232,240,0.7)"
        aria-label="İletildi"
        role="img"
      >
        <path d="M1,0 L5,0 L11,18 L7,18 Z" />
        <path d="M10,0 L14,0 L20,18 L16,18 Z" />
        <path d="M19,0 L23,0 L29,18 L25,18 Z" />
      </svg>
    );
  }

  // "sent"
  return (
    <svg
      width={size}
      height={size * 0.6}
      viewBox="0 0 22 18"
      fill="rgba(232,232,240,0.3)"
      aria-label="Gönderildi"
      role="img"
    >
      <path d="M1,0 L5,0 L12,18 L8,18 Z" />
      <path d="M11,0 L15,0 L22,18 L18,18 Z" />
    </svg>
  );
}

/** Map backend status string → MessageStatusType */
export function toStatusType(status: string): MessageStatusType {
  if (status === "read") return "seen";
  if (status === "delivered") return "delivered";
  return "sent";
}
