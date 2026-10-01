import type { Ctx } from "../main";
import { Shell } from "../shell";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { afterPaint, FrameRecorder, phase, sleep } from "../lib/metrics";

// Baseline: app shell with a few shadcn components, then 3 s idle.
export async function run({ root }: Ctx) {
  const t0 = performance.now();
  root.render(
    <Shell title="idle">
      <div className="p-6">
        <Card className="max-w-md">
          <CardHeader><CardTitle>Idle baseline</CardTitle></CardHeader>
          <CardContent className="flex gap-2"><Button>Run</Button><Button variant="outline">Cancel</Button></CardContent>
        </Card>
      </div>
    </Shell>,
  );
  const painted = (await afterPaint()) - t0;
  const fr = new FrameRecorder();
  fr.start();
  await phase("idle", () => sleep(3000));
  return { renderMs: painted, idleFrames: fr.stop() };
}
