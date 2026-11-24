import { GitHubMark, GoogleMark } from "@/components/ui/brand-icons";
import { buttonVariants } from "@/components/ui/button";
import { PROVIDER_LABELS, type Provider } from "@/lib/providers";
import { cn } from "@/lib/utils";

const MARKS: Record<Provider, React.ReactNode> = {
  google: <GoogleMark />,
  github: <GitHubMark />,
};

/**
 * "Continue with ..." links, one per configured provider. Plain links, not buttons or router
 * links: signing in is a full navigation to the API through the bridge, which answers with a
 * redirect to the provider, so there is nothing to prefetch and nothing for script to get wrong.
 */
export function ProviderButtons({ providers }: { providers: Provider[] }) {
  if (providers.length === 0) return null;
  return (
    <div className="flex flex-col gap-2" role="group" aria-label="Sign in with a provider">
      {providers.map((p) => (
        <a
          key={p}
          href={`/api/v1/auth/${p}/start`}
          className={cn(buttonVariants({ variant: "secondary", size: "lg" }), "w-full")}
        >
          {MARKS[p]}
          Continue with {PROVIDER_LABELS[p]}
        </a>
      ))}
    </div>
  );
}
