interface SettingsWork {
  workers: number;
  maxJobs: number;
  archiveBytes: number;
  archiveEntries: number;
  archiveDepth: number;
  archiveSeconds: number;
  reserveBytes: number;
  artifactBytes: number;
}
interface ISettings {
  work: SettingsWork;
  signup: boolean;
  createUserDir: boolean;
  hideLoginButton: boolean;
  minimumPasswordLength: number;
  userHomeBasePath: string;
  defaults: SettingsDefaults;
  authMethod: string;
  rules: any[];
  branding: SettingsBranding;
  tus: SettingsTus;
  collabora: SettingsCollabora;
  convertx: SettingsConvertX;
  erugo: SettingsErugo;
  clamav: SettingsClamAV;
  shell: string[];
  commands: SettingsCommand;
}

interface SettingsDefaults {
  scope: string;
  locale: string;
  viewMode: ViewModeType;
  singleClick: boolean;
  redirectAfterCopyMove: boolean;
  sorting: Sorting;
  perm: Permissions;
  commands: any[];
  hideDotfiles: boolean;
  dateFormat: boolean;
  aceEditorTheme: string;
}

interface SettingsBranding {
  name: string;
  disableExternal: boolean;
  disableUsedPercentage: boolean;
  files: string;
  theme: UserTheme;
  color: string;
}

interface SettingsTus {
  retentionHours: number;
  concurrentUploads: number;
  maxActiveUploads: number;
  maxPendingBytes: number;
  reserveBytes: number;
  chunkSize: number;
  retryCount: number;
}

interface SettingsCollabora {
  configured: boolean;
  enabled: boolean;
  url: string;
  publicURL: string;
  internalURL: string;
  wopiSecret: string;
  tokenTTL: string;
}

interface SettingsConvertX {
  connectTimeout?: string;
  uploadTimeout?: string;
  processTimeout?: string;
  downloadTimeout?: string;
  configured: boolean;
  enabled: boolean;
  url: string;
  apiKey: string;
  timeout: string;
}

interface SettingsErugo {
  configured: boolean;
  enabled: boolean;
  url: string;
  apiKey: string;
  shareEndpoint: string;
  reverseShareEndpoint: string;
  defaultExpiryHours: number;
  timeout: string;
  uploadTimeout: string;
}

interface SettingsClamAV {
  automaticMaxBytes: number;
  oversizePolicy: "skip" | "scan" | "block";
  scanWorkers: number;
  scanTimeout: string;
  enabled: boolean;
  url: string;
  scanDepth: number;
}

interface ClamAVTestResponse {
  status: string;
  message: string;
}

interface SettingsCommand {
  after_copy?: string[];
  after_delete?: string[];
  after_rename?: string[];
  after_save?: string[];
  after_upload?: string[];
  before_copy?: string[];
  before_delete?: string[];
  before_rename?: string[];
  before_save?: string[];
  before_upload?: string[];
}

interface SettingsUnit {
  KB: number;
  MB: number;
  GB: number;
  TB: number;
}
