import { AlertTriangle, CheckCircle2, XCircle } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { Overall, ServiceStatus } from "@/lib/status-schema";
import type { ServiceView, StatusView } from "@/lib/status-view";

type Tone = "ok" | "warn" | "bad";

const OVERALL: Record<Overall, { text: string; tone: Tone }> = {
  operational: { text: "All systems operational", tone: "ok" },
  degraded: { text: "Some services are degraded", tone: "warn" },
  unavailable: { text: "Service unavailable", tone: "bad" },
};

const SERVICE: Record<ServiceStatus, { text: string; tone: Tone }> = {
  ok: { text: "Operational", tone: "ok" },
  down: { text: "Down", tone: "bad" },
  unreachable: { text: "Unreachable", tone: "bad" },
  unconfigured: { text: "Not configured", tone: "warn" },
};

function ToneIcon({ tone }: { tone: Tone }) {
  const props = { className: "size-4", "aria-hidden": true } as const;
  if (tone === "ok") return <CheckCircle2 {...props} />;
  if (tone === "warn") return <AlertTriangle {...props} />;
  return <XCircle {...props} />;
}

// Status is always text plus an icon, never colour alone.
function StatusBadge({ text, tone }: { text: string; tone: Tone }) {
  return (
    <Badge tone={tone}>
      <ToneIcon tone={tone} />
      {text}
    </Badge>
  );
}

function ServiceCard({ title, service }: { title: string; service: ServiceView }) {
  const s = SERVICE[service.status];
  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between">
        <CardTitle>{title}</CardTitle>
        <StatusBadge text={s.text} tone={s.tone} />
      </CardHeader>
      {service.checks !== null && service.checks.length > 0 && (
        <CardContent>
          <dl className="grid grid-cols-[1fr_auto] gap-x-4 gap-y-1 text-sm">
            {service.checks.map((c) => (
              <div key={c.name} className="contents">
                <dt className="text-muted-foreground capitalize">{c.name}</dt>
                <dd>{c.state === "ok" ? "Operational" : "Down"}</dd>
              </div>
            ))}
          </dl>
        </CardContent>
      )}
    </Card>
  );
}

export function StatusPanel({ view }: { view: StatusView }) {
  if (view.kind !== "report") {
    const text =
      view.kind === "api_unreachable"
        ? "Can't reach the Readmely API"
        : "Received an invalid response from the API";
    return (
      <section aria-labelledby="overall" className="flex flex-col gap-4">
        <h2 id="overall" className="flex items-center gap-2 text-xl font-semibold">
          <StatusBadge
            text={view.kind === "api_unreachable" ? "Unreachable" : "Invalid"}
            tone="bad"
          />
          <span>{text}</span>
        </h2>
        <p className="text-muted-foreground text-sm">
          Service details are unavailable right now. Try again in a moment.
        </p>
      </section>
    );
  }

  const o = OVERALL[view.overall];
  return (
    <section aria-labelledby="overall" className="flex flex-col gap-4">
      <h2 id="overall" className="flex items-center gap-2 text-xl font-semibold">
        <StatusBadge
          text={o.tone === "ok" ? "OK" : o.tone === "warn" ? "Degraded" : "Down"}
          tone={o.tone}
        />
        <span>{o.text}</span>
      </h2>
      <div className="grid gap-4 sm:grid-cols-2">
        <ServiceCard title="API" service={view.api} />
        <ServiceCard title="AI service" service={view.ai} />
      </div>
    </section>
  );
}
