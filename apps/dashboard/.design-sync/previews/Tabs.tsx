import { Tabs } from '@job-finder/dashboard';

const noop = () => {};

const workspaceTabs = [
  { id: 'resume', label: 'Generated resume' },
  { id: 'job', label: 'Job description' },
  { id: 'settings', label: 'Settings' },
];

const profileTabs = [
  { id: 'details', label: 'Details' },
  { id: 'experience', label: 'Experience' },
  { id: 'skills', label: 'Skills' },
  { id: 'config', label: 'Config' },
];

export const Segmented = () => (
  <Tabs aria-label="Generate workspace view" tabs={workspaceTabs} active="resume" onChange={noop} />
);

export const Underline = () => (
  <Tabs aria-label="Profile" variant="underline" tabs={profileTabs} active="experience" onChange={noop} />
);
