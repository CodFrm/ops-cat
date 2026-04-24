export namespace ai {
	
	export class MentionedAsset {
	    assetId: number;
	    name: string;
	    type: string;
	    host: string;
	    groupPath: string;
	
	    static createFrom(source: any = {}) {
	        return new MentionedAsset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.assetId = source["assetId"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.host = source["host"];
	        this.groupPath = source["groupPath"];
	    }
	}
	export class TabInfo {
	    type: string;
	    assetId: number;
	    assetName: string;
	
	    static createFrom(source: any = {}) {
	        return new TabInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.assetId = source["assetId"];
	        this.assetName = source["assetName"];
	    }
	}
	export class AIContext {
	    openTabs: TabInfo[];
	    mentionedAssets: MentionedAsset[];
	
	    static createFrom(source: any = {}) {
	        return new AIContext(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.openTabs = this.convertValues(source["openTabs"], TabInfo);
	        this.mentionedAssets = this.convertValues(source["mentionedAssets"], MentionedAsset);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ApprovalItem {
	    type: string;
	    asset_id: number;
	    asset_name: string;
	    group_id?: number;
	    group_name?: string;
	    command: string;
	    detail?: string;
	
	    static createFrom(source: any = {}) {
	        return new ApprovalItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.asset_id = source["asset_id"];
	        this.asset_name = source["asset_name"];
	        this.group_id = source["group_id"];
	        this.group_name = source["group_name"];
	        this.command = source["command"];
	        this.detail = source["detail"];
	    }
	}
	export class ApprovalResponse {
	    decision: string;
	    edited_items?: ApprovalItem[];
	
	    static createFrom(source: any = {}) {
	        return new ApprovalResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.decision = source["decision"];
	        this.edited_items = this.convertValues(source["edited_items"], ApprovalItem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ToolCall {
	    id: string;
	    type: string;
	    // Go type: struct { Name string "json:\"name\""; Arguments string "json:\"arguments\"" }
	    function: any;
	
	    static createFrom(source: any = {}) {
	        return new ToolCall(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.function = this.convertValues(source["function"], Object);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Message {
	    role: string;
	    content: string;
	    thinking?: string;
	    tool_calls?: ToolCall[];
	    tool_call_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.thinking = source["thinking"];
	        this.tool_calls = this.convertValues(source["tool_calls"], ToolCall);
	        this.tool_call_id = source["tool_call_id"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	

}

export namespace app {
	
	export class AIModelInfo {
	    id: string;
	    maxOutputTokens: number;
	    contextWindow: number;
	
	    static createFrom(source: any = {}) {
	        return new AIModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.maxOutputTokens = source["maxOutputTokens"];
	        this.contextWindow = source["contextWindow"];
	    }
	}
	export class AIProviderInfo {
	    id: number;
	    name: string;
	    type: string;
	    apiBase: string;
	    apiKey: string;
	    maskedApiKey: string;
	    model: string;
	    maxOutputTokens: number;
	    contextWindow: number;
	    isActive: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AIProviderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.apiBase = source["apiBase"];
	        this.apiKey = source["apiKey"];
	        this.maskedApiKey = source["maskedApiKey"];
	        this.model = source["model"];
	        this.maxOutputTokens = source["maxOutputTokens"];
	        this.contextWindow = source["contextWindow"];
	        this.isActive = source["isActive"];
	    }
	}
	export class AssetTypeInfo {
	    type: string;
	    extensionName?: string;
	    displayName: string;
	    sshTunnel: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AssetTypeInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.extensionName = source["extensionName"];
	        this.displayName = source["displayName"];
	        this.sshTunnel = source["sshTunnel"];
	    }
	}
	export class AuditLogListResult {
	    items: audit_entity.AuditLog[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new AuditLogListResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], audit_entity.AuditLog);
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BugReportInfo {
	    version: string;
	    commit: string;
	    os: string;
	    arch: string;
	    osLabel: string;
	
	    static createFrom(source: any = {}) {
	        return new BugReportInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.commit = source["commit"];
	        this.os = source["os"];
	        this.arch = source["arch"];
	        this.osLabel = source["osLabel"];
	    }
	}
	export class ConversationDisplayMessage {
	    role: string;
	    content: string;
	    blocks: conversation_entity.ContentBlock[];
	    mentions: conversation_entity.MentionRef[];
	    tokenUsage?: conversation_entity.TokenUsage;
	
	    static createFrom(source: any = {}) {
	        return new ConversationDisplayMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.blocks = this.convertValues(source["blocks"], conversation_entity.ContentBlock);
	        this.mentions = this.convertValues(source["mentions"], conversation_entity.MentionRef);
	        this.tokenUsage = this.convertValues(source["tokenUsage"], conversation_entity.TokenUsage);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RuleWithStatus {
	    id: number;
	    configId: number;
	    type: string;
	    localHost: string;
	    localPort: number;
	    remoteHost: string;
	    remotePort: number;
	    createtime: number;
	    updatetime: number;
	    status: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new RuleWithStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.configId = source["configId"];
	        this.type = source["type"];
	        this.localHost = source["localHost"];
	        this.localPort = source["localPort"];
	        this.remoteHost = source["remoteHost"];
	        this.remotePort = source["remotePort"];
	        this.createtime = source["createtime"];
	        this.updatetime = source["updatetime"];
	        this.status = source["status"];
	        this.error = source["error"];
	    }
	}
	export class ForwardConfigWithStatus {
	    id: number;
	    name: string;
	    assetId: number;
	    createtime: number;
	    updatetime: number;
	    assetName: string;
	    rules: RuleWithStatus[];
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new ForwardConfigWithStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.assetId = source["assetId"];
	        this.createtime = source["createtime"];
	        this.updatetime = source["updatetime"];
	        this.assetName = source["assetName"];
	        this.rules = this.convertValues(source["rules"], RuleWithStatus);
	        this.status = source["status"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportFileInfo {
	    filePath: string;
	    encrypted: boolean;
	    summary?: backup_svc.BackupSummary;
	
	    static createFrom(source: any = {}) {
	        return new ImportFileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filePath = source["filePath"];
	        this.encrypted = source["encrypted"];
	        this.summary = this.convertValues(source["summary"], backup_svc.BackupSummary);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LocalSSHKeyInfo {
	    path: string;
	    keyType: string;
	    fingerprint: string;
	    isEncrypted: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LocalSSHKeyInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.keyType = source["keyType"];
	        this.fingerprint = source["fingerprint"];
	        this.isEncrypted = source["isEncrypted"];
	    }
	}
	export class OpsctlInfo {
	    installed: boolean;
	    path: string;
	    version: string;
	    embedded: boolean;
	
	    static createFrom(source: any = {}) {
	        return new OpsctlInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.path = source["path"];
	        this.version = source["version"];
	        this.embedded = source["embedded"];
	    }
	}
	export class PolicyTestRequest {
	    policyType: string;
	    policyJSON: string;
	    command: string;
	    assetID: number;
	    groupID: number;
	
	    static createFrom(source: any = {}) {
	        return new PolicyTestRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.policyType = source["policyType"];
	        this.policyJSON = source["policyJSON"];
	        this.command = source["command"];
	        this.assetID = source["assetID"];
	        this.groupID = source["groupID"];
	    }
	}
	export class PolicyTestResult {
	    decision: string;
	    matchedPattern: string;
	    matchedSource: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new PolicyTestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.decision = source["decision"];
	        this.matchedPattern = source["matchedPattern"];
	        this.matchedSource = source["matchedSource"];
	        this.message = source["message"];
	    }
	}
	
	export class SSHConnectRequest {
	    assetId: number;
	    password: string;
	    key: string;
	    cols: number;
	    rows: number;
	
	    static createFrom(source: any = {}) {
	        return new SSHConnectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.assetId = source["assetId"];
	        this.password = source["password"];
	        this.key = source["key"];
	        this.cols = source["cols"];
	        this.rows = source["rows"];
	    }
	}
	export class SkillTarget {
	    name: string;
	    installed: boolean;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new SkillTarget(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.installed = source["installed"];
	        this.path = source["path"];
	    }
	}

}

export namespace asset_entity {
	
	export class Asset {
	    ID: number;
	    Name: string;
	    Type: string;
	    GroupID: number;
	    Icon: string;
	    Tags: string;
	    Description: string;
	    Config: string;
	    CmdPolicy: string;
	    SortOrder: number;
	    sshTunnelId: number;
	    extensionName?: string;
	    Status: number;
	    Createtime: number;
	    Updatetime: number;
	
	    static createFrom(source: any = {}) {
	        return new Asset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.Type = source["Type"];
	        this.GroupID = source["GroupID"];
	        this.Icon = source["Icon"];
	        this.Tags = source["Tags"];
	        this.Description = source["Description"];
	        this.Config = source["Config"];
	        this.CmdPolicy = source["CmdPolicy"];
	        this.SortOrder = source["SortOrder"];
	        this.sshTunnelId = source["sshTunnelId"];
	        this.extensionName = source["extensionName"];
	        this.Status = source["Status"];
	        this.Createtime = source["Createtime"];
	        this.Updatetime = source["Updatetime"];
	    }
	}

}

export namespace audit_entity {
	
	export class AuditLog {
	    ID: number;
	    Source: string;
	    ToolName: string;
	    AssetID: number;
	    AssetName: string;
	    Command: string;
	    Request: string;
	    Result: string;
	    Error: string;
	    Success: number;
	    ConversationID: number;
	    GrantSessionID: string;
	    SessionID: string;
	    Decision: string;
	    DecisionSource: string;
	    MatchedPattern: string;
	    Createtime: number;
	
	    static createFrom(source: any = {}) {
	        return new AuditLog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Source = source["Source"];
	        this.ToolName = source["ToolName"];
	        this.AssetID = source["AssetID"];
	        this.AssetName = source["AssetName"];
	        this.Command = source["Command"];
	        this.Request = source["Request"];
	        this.Result = source["Result"];
	        this.Error = source["Error"];
	        this.Success = source["Success"];
	        this.ConversationID = source["ConversationID"];
	        this.GrantSessionID = source["GrantSessionID"];
	        this.SessionID = source["SessionID"];
	        this.Decision = source["Decision"];
	        this.DecisionSource = source["DecisionSource"];
	        this.MatchedPattern = source["MatchedPattern"];
	        this.Createtime = source["Createtime"];
	    }
	}

}

export namespace audit_repo {
	
	export class SessionInfo {
	    session_id: string;
	    first_time: number;
	    last_time: number;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.first_time = source["first_time"];
	        this.last_time = source["last_time"];
	        this.count = source["count"];
	    }
	}

}

export namespace backup_svc {
	
	export class BackupSummary {
	    version: string;
	    exported_at: string;
	    encrypted: boolean;
	    includes_credentials: boolean;
	    asset_count: number;
	    group_count: number;
	    credential_count: number;
	    policy_group_count: number;
	    forward_count: number;
	    has_shortcuts: boolean;
	    has_custom_themes: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BackupSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.exported_at = source["exported_at"];
	        this.encrypted = source["encrypted"];
	        this.includes_credentials = source["includes_credentials"];
	        this.asset_count = source["asset_count"];
	        this.group_count = source["group_count"];
	        this.credential_count = source["credential_count"];
	        this.policy_group_count = source["policy_group_count"];
	        this.forward_count = source["forward_count"];
	        this.has_shortcuts = source["has_shortcuts"];
	        this.has_custom_themes = source["has_custom_themes"];
	    }
	}
	export class DeviceFlowInfo {
	    deviceCode: string;
	    userCode: string;
	    verificationUri: string;
	    expiresIn: number;
	    interval: number;
	
	    static createFrom(source: any = {}) {
	        return new DeviceFlowInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.deviceCode = source["deviceCode"];
	        this.userCode = source["userCode"];
	        this.verificationUri = source["verificationUri"];
	        this.expiresIn = source["expiresIn"];
	        this.interval = source["interval"];
	    }
	}
	export class ExportOptions {
	    asset_ids: number[];
	    include_credentials: boolean;
	    include_forwards: boolean;
	    include_policy_groups: boolean;
	    shortcuts?: string;
	    custom_themes?: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.asset_ids = source["asset_ids"];
	        this.include_credentials = source["include_credentials"];
	        this.include_forwards = source["include_forwards"];
	        this.include_policy_groups = source["include_policy_groups"];
	        this.shortcuts = source["shortcuts"];
	        this.custom_themes = source["custom_themes"];
	    }
	}
	export class GistInfo {
	    id: string;
	    description: string;
	    updatedAt: string;
	    htmlUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new GistInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.description = source["description"];
	        this.updatedAt = source["updatedAt"];
	        this.htmlUrl = source["htmlUrl"];
	    }
	}
	export class GitHubUser {
	    login: string;
	    avatarUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new GitHubUser(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.login = source["login"];
	        this.avatarUrl = source["avatarUrl"];
	    }
	}
	export class ImportOptions {
	    import_assets: boolean;
	    import_credentials: boolean;
	    import_forwards: boolean;
	    import_policy_groups: boolean;
	    import_shortcuts: boolean;
	    import_themes: boolean;
	    mode: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.import_assets = source["import_assets"];
	        this.import_credentials = source["import_credentials"];
	        this.import_forwards = source["import_forwards"];
	        this.import_policy_groups = source["import_policy_groups"];
	        this.import_shortcuts = source["import_shortcuts"];
	        this.import_themes = source["import_themes"];
	        this.mode = source["mode"];
	    }
	}
	export class ImportResult {
	    assets_imported: number;
	    groups_imported: number;
	    credentials_imported: number;
	    policy_groups_imported: number;
	    forwards_imported: number;
	    shortcuts?: string;
	    custom_themes?: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.assets_imported = source["assets_imported"];
	        this.groups_imported = source["groups_imported"];
	        this.credentials_imported = source["credentials_imported"];
	        this.policy_groups_imported = source["policy_groups_imported"];
	        this.forwards_imported = source["forwards_imported"];
	        this.shortcuts = source["shortcuts"];
	        this.custom_themes = source["custom_themes"];
	    }
	}

}

export namespace conversation_entity {
	
	export class ContentBlock {
	    type: string;
	    content: string;
	    toolName?: string;
	    toolInput?: string;
	    status?: string;
	
	    static createFrom(source: any = {}) {
	        return new ContentBlock(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.content = source["content"];
	        this.toolName = source["toolName"];
	        this.toolInput = source["toolInput"];
	        this.status = source["status"];
	    }
	}
	export class Conversation {
	    ID: number;
	    Title: string;
	    ProviderType: string;
	    Model: string;
	    ProviderID: number;
	    SessionData: string;
	    WorkDir: string;
	    Status: number;
	    Createtime: number;
	    Updatetime: number;
	
	    static createFrom(source: any = {}) {
	        return new Conversation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Title = source["Title"];
	        this.ProviderType = source["ProviderType"];
	        this.Model = source["Model"];
	        this.ProviderID = source["ProviderID"];
	        this.SessionData = source["SessionData"];
	        this.WorkDir = source["WorkDir"];
	        this.Status = source["Status"];
	        this.Createtime = source["Createtime"];
	        this.Updatetime = source["Updatetime"];
	    }
	}
	export class MentionRef {
	    assetId: number;
	    name: string;
	    start: number;
	    end: number;
	
	    static createFrom(source: any = {}) {
	        return new MentionRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.assetId = source["assetId"];
	        this.name = source["name"];
	        this.start = source["start"];
	        this.end = source["end"];
	    }
	}
	export class TokenUsage {
	    inputTokens?: number;
	    outputTokens?: number;
	    cacheCreationTokens?: number;
	    cacheReadTokens?: number;
	
	    static createFrom(source: any = {}) {
	        return new TokenUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inputTokens = source["inputTokens"];
	        this.outputTokens = source["outputTokens"];
	        this.cacheCreationTokens = source["cacheCreationTokens"];
	        this.cacheReadTokens = source["cacheReadTokens"];
	    }
	}

}

export namespace credential_entity {
	
	export class Credential {
	    id: number;
	    name: string;
	    type: string;
	    username?: string;
	    publicKey?: string;
	    keyType?: string;
	    keySize?: number;
	    fingerprint?: string;
	    comment?: string;
	    description?: string;
	    createtime: number;
	    updatetime: number;
	
	    static createFrom(source: any = {}) {
	        return new Credential(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.username = source["username"];
	        this.publicKey = source["publicKey"];
	        this.keyType = source["keyType"];
	        this.keySize = source["keySize"];
	        this.fingerprint = source["fingerprint"];
	        this.comment = source["comment"];
	        this.description = source["description"];
	        this.createtime = source["createtime"];
	        this.updatetime = source["updatetime"];
	    }
	}

}

export namespace extension {
	
	export class I18nName {
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new I18nName(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	    }
	}
	export class AssetTypeDef {
	    type: string;
	    i18n: I18nName;
	    configSchema: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new AssetTypeDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.i18n = this.convertValues(source["i18n"], I18nName);
	        this.configSchema = source["configSchema"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class HTTPCapability {
	    allowlist: string[];
	
	    static createFrom(source: any = {}) {
	        return new HTTPCapability(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.allowlist = source["allowlist"];
	    }
	}
	export class FSCapability {
	    read: string[];
	    write: string[];
	
	    static createFrom(source: any = {}) {
	        return new FSCapability(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.read = source["read"];
	        this.write = source["write"];
	    }
	}
	export class Capabilities {
	    fs: FSCapability;
	    http: HTTPCapability;
	    credentials: string;
	    tunnel: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Capabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fs = this.convertValues(source["fs"], FSCapability);
	        this.http = this.convertValues(source["http"], HTTPCapability);
	        this.credentials = source["credentials"];
	        this.tunnel = source["tunnel"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class PageDef {
	    id: string;
	    slot?: string;
	    i18n: I18nName;
	    component: string;
	
	    static createFrom(source: any = {}) {
	        return new PageDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.slot = source["slot"];
	        this.i18n = this.convertValues(source["i18n"], I18nName);
	        this.component = source["component"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FrontendDef {
	    entry: string;
	    styles: string;
	    pages: PageDef[];
	
	    static createFrom(source: any = {}) {
	        return new FrontendDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entry = source["entry"];
	        this.styles = source["styles"];
	        this.pages = this.convertValues(source["pages"], PageDef);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class I18nDesc {
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new I18nDesc(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.description = source["description"];
	    }
	}
	
	export class I18nNameDesc {
	    name: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new I18nNameDesc(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	    }
	}
	export class SeedSnippetDef {
	    key: string;
	    name: string;
	    category: string;
	    content: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new SeedSnippetDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.name = source["name"];
	        this.category = source["category"];
	        this.content = source["content"];
	        this.description = source["description"];
	    }
	}
	export class SnippetCategoryDef {
	    id: string;
	    assetType: string;
	    i18n: I18nName;
	
	    static createFrom(source: any = {}) {
	        return new SnippetCategoryDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.assetType = source["assetType"];
	        this.i18n = this.convertValues(source["i18n"], I18nName);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SnippetsDef {
	    categories: SnippetCategoryDef[];
	    seed: SeedSnippetDef[];
	
	    static createFrom(source: any = {}) {
	        return new SnippetsDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.categories = this.convertValues(source["categories"], SnippetCategoryDef);
	        this.seed = this.convertValues(source["seed"], SeedSnippetDef);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PolicyGroupDef {
	    id: string;
	    i18n: I18nNameDesc;
	    policy: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new PolicyGroupDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.i18n = this.convertValues(source["i18n"], I18nNameDesc);
	        this.policy = source["policy"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PoliciesDef {
	    type: string;
	    actions: string[];
	    groups: PolicyGroupDef[];
	    default: string[];
	
	    static createFrom(source: any = {}) {
	        return new PoliciesDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.actions = source["actions"];
	        this.groups = this.convertValues(source["groups"], PolicyGroupDef);
	        this.default = source["default"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ToolDef {
	    name: string;
	    i18n: I18nDesc;
	    parameters: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new ToolDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.i18n = this.convertValues(source["i18n"], I18nDesc);
	        this.parameters = source["parameters"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ManifestBackend {
	    runtime: string;
	    binary: string;
	
	    static createFrom(source: any = {}) {
	        return new ManifestBackend(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runtime = source["runtime"];
	        this.binary = source["binary"];
	    }
	}
	export class ManifestI18n {
	    displayName: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new ManifestI18n(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.displayName = source["displayName"];
	        this.description = source["description"];
	    }
	}
	export class Manifest {
	    name: string;
	    version: string;
	    icon: string;
	    minAppVersion: string;
	    hostABI: string;
	    capabilities: Capabilities;
	    i18n: ManifestI18n;
	    backend: ManifestBackend;
	    assetTypes: AssetTypeDef[];
	    tools: ToolDef[];
	    policies: PoliciesDef;
	    frontend: FrontendDef;
	    snippets: SnippetsDef;
	
	    static createFrom(source: any = {}) {
	        return new Manifest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.icon = source["icon"];
	        this.minAppVersion = source["minAppVersion"];
	        this.hostABI = source["hostABI"];
	        this.capabilities = this.convertValues(source["capabilities"], Capabilities);
	        this.i18n = this.convertValues(source["i18n"], ManifestI18n);
	        this.backend = this.convertValues(source["backend"], ManifestBackend);
	        this.assetTypes = this.convertValues(source["assetTypes"], AssetTypeDef);
	        this.tools = this.convertValues(source["tools"], ToolDef);
	        this.policies = this.convertValues(source["policies"], PoliciesDef);
	        this.frontend = this.convertValues(source["frontend"], FrontendDef);
	        this.snippets = this.convertValues(source["snippets"], SnippetsDef);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	
	

}

export namespace extension_svc {
	
	export class ExtensionInfo {
	    name: string;
	    version: string;
	    icon: string;
	    displayName: string;
	    description: string;
	    enabled: boolean;
	    manifest?: extension.Manifest;
	
	    static createFrom(source: any = {}) {
	        return new ExtensionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.icon = source["icon"];
	        this.displayName = source["displayName"];
	        this.description = source["description"];
	        this.enabled = source["enabled"];
	        this.manifest = this.convertValues(source["manifest"], extension.Manifest);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace forward_entity {
	
	export class ForwardConfig {
	    id: number;
	    name: string;
	    assetId: number;
	    createtime: number;
	    updatetime: number;
	
	    static createFrom(source: any = {}) {
	        return new ForwardConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.assetId = source["assetId"];
	        this.createtime = source["createtime"];
	        this.updatetime = source["updatetime"];
	    }
	}
	export class ForwardRule {
	    id: number;
	    configId: number;
	    type: string;
	    localHost: string;
	    localPort: number;
	    remoteHost: string;
	    remotePort: number;
	    createtime: number;
	    updatetime: number;
	
	    static createFrom(source: any = {}) {
	        return new ForwardRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.configId = source["configId"];
	        this.type = source["type"];
	        this.localHost = source["localHost"];
	        this.localPort = source["localPort"];
	        this.remoteHost = source["remoteHost"];
	        this.remotePort = source["remotePort"];
	        this.createtime = source["createtime"];
	        this.updatetime = source["updatetime"];
	    }
	}

}

export namespace group_entity {
	
	export class Group {
	    ID: number;
	    Name: string;
	    ParentID: number;
	    Icon: string;
	    Description: string;
	    CmdPolicy: string;
	    QryPolicy: string;
	    RdsPolicy: string;
	    MgoPolicy: string;
	    SortOrder: number;
	    Createtime: number;
	    Updatetime: number;
	
	    static createFrom(source: any = {}) {
	        return new Group(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.ParentID = source["ParentID"];
	        this.Icon = source["Icon"];
	        this.Description = source["Description"];
	        this.CmdPolicy = source["CmdPolicy"];
	        this.QryPolicy = source["QryPolicy"];
	        this.RdsPolicy = source["RdsPolicy"];
	        this.MgoPolicy = source["MgoPolicy"];
	        this.SortOrder = source["SortOrder"];
	        this.Createtime = source["Createtime"];
	        this.Updatetime = source["Updatetime"];
	    }
	}

}

export namespace import_svc {
	
	export class ImportError {
	    name: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportError(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.reason = source["reason"];
	    }
	}
	export class ImportResult {
	    total: number;
	    success: number;
	    skipped: number;
	    failed: number;
	    errors: ImportError[];
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.success = source["success"];
	        this.skipped = source["skipped"];
	        this.failed = source["failed"];
	        this.errors = this.convertValues(source["errors"], ImportError);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PreviewGroup {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new PreviewGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class PreviewItem {
	    index: number;
	    name: string;
	    host: string;
	    port: number;
	    username: string;
	    authType: string;
	    groupId: string;
	    exists: boolean;
	    hasPassword: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PreviewItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.authType = source["authType"];
	        this.groupId = source["groupId"];
	        this.exists = source["exists"];
	        this.hasPassword = source["hasPassword"];
	    }
	}
	export class PreviewResult {
	    groups: PreviewGroup[];
	    items: PreviewItem[];
	    hasVault: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PreviewResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.groups = this.convertValues(source["groups"], PreviewGroup);
	        this.items = this.convertValues(source["items"], PreviewItem);
	        this.hasVault = source["hasVault"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace policy_group_entity {
	
	export class PolicyGroup {
	    id: number;
	    name: string;
	    description: string;
	    policyType: string;
	    policy: string;
	    createtime: number;
	    updatetime: number;
	
	    static createFrom(source: any = {}) {
	        return new PolicyGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.policyType = source["policyType"];
	        this.policy = source["policy"];
	        this.createtime = source["createtime"];
	        this.updatetime = source["updatetime"];
	    }
	}
	export class PolicyGroupItem {
	    id: string;
	    name: string;
	    description: string;
	    policyType: string;
	    policy: string;
	    builtin: boolean;
	    extensionName?: string;
	    createtime: number;
	    updatetime: number;
	
	    static createFrom(source: any = {}) {
	        return new PolicyGroupItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.policyType = source["policyType"];
	        this.policy = source["policy"];
	        this.builtin = source["builtin"];
	        this.extensionName = source["extensionName"];
	        this.createtime = source["createtime"];
	        this.updatetime = source["updatetime"];
	    }
	}

}

export namespace sftp_svc {
	
	export class FileEntry {
	    name: string;
	    size: number;
	    isDir: boolean;
	    modTime: number;
	
	    static createFrom(source: any = {}) {
	        return new FileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.size = source["size"];
	        this.isDir = source["isDir"];
	        this.modTime = source["modTime"];
	    }
	}

}

export namespace snippet_entity {
	
	export class Snippet {
	    ID: number;
	    Name: string;
	    Category: string;
	    Content: string;
	    Description: string;
	    LastAssetIDs: string;
	    Source: string;
	    SourceRef: string;
	    UseCount: number;
	    // Go type: time
	    LastUsedAt?: any;
	    Status: number;
	    // Go type: time
	    CreatedAt: any;
	    // Go type: time
	    UpdatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Snippet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.Category = source["Category"];
	        this.Content = source["Content"];
	        this.Description = source["Description"];
	        this.LastAssetIDs = source["LastAssetIDs"];
	        this.Source = source["Source"];
	        this.SourceRef = source["SourceRef"];
	        this.UseCount = source["UseCount"];
	        this.LastUsedAt = this.convertValues(source["LastUsedAt"], null);
	        this.Status = source["Status"];
	        this.CreatedAt = this.convertValues(source["CreatedAt"], null);
	        this.UpdatedAt = this.convertValues(source["UpdatedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace snippet_svc {
	
	export class Category {
	    id: string;
	    assetType: string;
	    label: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new Category(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.assetType = source["assetType"];
	        this.label = source["label"];
	        this.source = source["source"];
	    }
	}
	export class CreateReq {
	    name: string;
	    category: string;
	    content: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateReq(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.category = source["category"];
	        this.content = source["content"];
	        this.description = source["description"];
	    }
	}
	export class ListReq {
	    categories: string[];
	    keyword: string;
	    limit: number;
	    offset: number;
	    orderBy: string;
	
	    static createFrom(source: any = {}) {
	        return new ListReq(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.categories = source["categories"];
	        this.keyword = source["keyword"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	        this.orderBy = source["orderBy"];
	    }
	}
	export class UpdateReq {
	    id: number;
	    name: string;
	    content: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateReq(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.content = source["content"];
	        this.description = source["description"];
	    }
	}

}

export namespace sshpool {
	
	export class PoolEntryInfo {
	    asset_id: number;
	    ref_count: number;
	    last_used: number;
	
	    static createFrom(source: any = {}) {
	        return new PoolEntryInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.asset_id = source["asset_id"];
	        this.ref_count = source["ref_count"];
	        this.last_used = source["last_used"];
	    }
	}

}

export namespace status {
	
	export class Entry {
	    level: string;
	    source: string;
	    message: string;
	    detail: string;
	    // Go type: time
	    time: any;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.source = source["source"];
	        this.message = source["message"];
	        this.detail = source["detail"];
	        this.time = this.convertValues(source["time"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace update_svc {
	
	export class MirrorInfo {
	    id: string;
	    name: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new MirrorInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.url = source["url"];
	    }
	}
	export class UpdateInfo {
	    hasUpdate: boolean;
	    currentVersion: string;
	    latestVersion: string;
	    releaseNotes: string;
	    releaseURL: string;
	    publishedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hasUpdate = source["hasUpdate"];
	        this.currentVersion = source["currentVersion"];
	        this.latestVersion = source["latestVersion"];
	        this.releaseNotes = source["releaseNotes"];
	        this.releaseURL = source["releaseURL"];
	        this.publishedAt = source["publishedAt"];
	    }
	}

}

