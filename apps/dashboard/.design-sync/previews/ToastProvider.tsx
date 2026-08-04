import { Button, Surface, ToastProvider } from '@job-finder/dashboard';

// ToastProvider renders no visible chrome of its own — it mounts the HeroUI toast
// region and subscribes to the app's toast bus. Toasts appear only in response to a
// runtime emit, which a static card cannot trigger, so these cells show the provider
// in its real position: wrapping the app content it serves.
export const WrappingAppContent = () => (
  <ToastProvider>
    <Surface>
      <h3 className="text-base font-semibold text-foreground">Application saved</h3>
      <p className="mt-2 text-sm text-muted">
        Mount <code className="text-foreground">ToastProvider</code> once at the app root. Anything below it
        can raise a toast through <code className="text-foreground">useToast()</code>; the provider owns the
        toast region and the subscription to the toast bus.
      </p>
      <div className="mt-4 flex justify-end">
        <Button variant="primary">Save</Button>
      </div>
    </Surface>
  </ToastProvider>
);
