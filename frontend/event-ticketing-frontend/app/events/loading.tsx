/** Streamed while the page awaits its data. A skeleton reserves the shape of the
 *  content, so nothing jumps when it lands -- which a centred spinner does not. */
export default function Loading() {
    return (
        <div className="flex flex-col gap-6">
            <div className="flex flex-col gap-2">
                <div className="h-8 w-56 animate-pulse rounded bg-surface-sunken" />
                <div className="h-4 w-72 animate-pulse rounded bg-surface-sunken" />
            </div>

            <ul className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
                {[0, 1, 2].map((i) => (
                    <li key={i} className="rounded-lg border border-border bg-surface p-5">
                        <div className="flex animate-pulse flex-col gap-3">
                            <div className="h-3 w-24 rounded bg-surface-sunken" />
                            <div className="h-5 w-3/4 rounded bg-surface-sunken" />
                            <div className="h-3 w-full rounded bg-surface-sunken" />
                            <div className="h-3 w-1/3 rounded bg-surface-sunken" />
                        </div>
                    </li>
                ))}
            </ul>
        </div>
    );
}
