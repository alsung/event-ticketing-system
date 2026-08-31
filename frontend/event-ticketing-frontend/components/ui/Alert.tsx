/** A form-level message. Field-level errors belong on the Field itself; this is
 *  for failures that belong to the whole submission, such as a rejected login. */
export function Alert({ children, tone = 'danger' }: {
    children: React.ReactNode;
    tone?: 'danger' | 'success';
}) {
    const tones = {
        danger: 'bg-danger-subtle text-danger',
        success: 'bg-success-subtle text-success',
    };
    return (
        <div role="alert" className={`rounded-md px-3.5 py-3 text-sm font-medium ${tones[tone]}`}>
            {children}
        </div>
    );
}
