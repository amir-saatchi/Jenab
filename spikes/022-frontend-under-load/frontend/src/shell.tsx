import type { ReactNode } from "react";
import { Badge } from "@/components/ui/badge";

export function Shell({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      <header className="flex h-10 shrink-0 items-center gap-2 border-b px-3 text-sm">
        <span className="font-semibold">Burrow SPIKE-022</span>
        <Badge variant="secondary">{title}</Badge>
      </header>
      <main className="min-h-0 flex-1">{children}</main>
    </div>
  );
}
