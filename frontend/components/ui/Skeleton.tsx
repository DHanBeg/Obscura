import { cn } from "@/lib/cn";

export function Skeleton({ className }: { className?: string }) {
  return (
    <div className={cn("shimmer rounded-2xl bg-raised", className)} />
  );
}

export function ConvSkeleton() {
  return (
    <div className="flex items-center gap-3 px-4 py-3.5">
      <Skeleton className="w-11 h-11 rounded-full flex-shrink-0" />
      <div className="flex-1 space-y-2">
        <Skeleton className="h-3.5 w-1/3" />
        <Skeleton className="h-3 w-2/3" />
      </div>
      <Skeleton className="h-3 w-8" />
    </div>
  );
}

export function MsgSkeleton({ mine }: { mine?: boolean }) {
  return (
    <div className={cn("flex", mine ? "justify-end" : "justify-start")}>
      <Skeleton className={cn("h-9 rounded-3xl", mine ? "w-40" : "w-52")} />
    </div>
  );
}
