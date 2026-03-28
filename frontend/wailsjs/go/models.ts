export namespace ai {
	
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
	
	    static createFrom(source: any = {}) {
	        return new AIContext(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.openTabs = this.convertValues(source["openTabs"], TabInfo);
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
	export class ConversationDisplayMessage {
	    role: string;
	    content: string;
	    blocks: conversation_entity.ContentBlock[];
	
	    static createFrom(source: any = {}) {
	        return new ConversationDisplayMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.blocks = this.convertValues(source["blocks"], conversation_entity.ContentBlock);
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
	export class ExtensionListItem {
	    name: string;
	    displayName: string;
	    displayName_zh: string;
	    version: string;
	    icon: string;
	    description: string;
	    description_zh: string;
	    assetTypes: extruntime.AssetTypeDef[];
	    pages: extruntime.PageDef[];
	    policyType?: string;
	    policyActions?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ExtensionListItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.displayName = source["displayName"];
	        this.displayName_zh = source["displayName_zh"];
	        this.version = source["version"];
	        this.icon = source["icon"];
	        this.description = source["description"];
	        this.description_zh = source["description_zh"];
	        this.assetTypes = this.convertValues(source["assetTypes"], extruntime.AssetTypeDef);
	        this.pages = this.convertValues(source["pages"], extruntime.PageDef);
	        this.policyType = source["policyType"];
	        this.policyActions = source["policyActions"];
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
	
	    static createFrom(source: any = {}) {
	        return new LocalSSHKeyInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.keyType = source["keyType"];
	        this.fingerprint = source["fingerprint"];
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

export namespace extruntime {
	
	export class AssetTypeDef {
	    type: string;
	    name: string;
	    name_zh: string;
	    namePlaceholder: string;
	    testConnection: boolean;
	    configSchema: number[];
	
	    static createFrom(source: any = {}) {
	        return new AssetTypeDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.name = source["name"];
	        this.name_zh = source["name_zh"];
	        this.namePlaceholder = source["namePlaceholder"];
	        this.testConnection = source["testConnection"];
	        this.configSchema = source["configSchema"];
	    }
	}
	export class PageDef {
	    id: string;
	    name: string;
	    name_zh: string;
	    component: string;
	
	    static createFrom(source: any = {}) {
	        return new PageDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.name_zh = source["name_zh"];
	        this.component = source["component"];
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
	    name_zh?: string;
	    description: string;
	    description_zh?: string;
	    policyType: string;
	    policy: string;
	    source: string;
	    createtime: number;
	    updatetime: number;
	
	    static createFrom(source: any = {}) {
	        return new PolicyGroupItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.name_zh = source["name_zh"];
	        this.description = source["description"];
	        this.description_zh = source["description_zh"];
	        this.policyType = source["policyType"];
	        this.policy = source["policy"];
	        this.source = source["source"];
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

