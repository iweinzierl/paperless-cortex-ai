class Session {
  final String token;
  final String username;
  final int createdAtMs;
  final int expiresAtMs;
  final int lastSeenAtMs;

  Session({
    required this.token,
    required this.username,
    required this.createdAtMs,
    required this.expiresAtMs,
    required this.lastSeenAtMs,
  });

  factory Session.fromJson(Map<String, dynamic> json) {
    return Session(
      token: _asString(json['token']) ?? '',
      username: _asString(json['username']) ?? '',
      createdAtMs: _asInt(json['created_at_ms']) ?? 0,
      expiresAtMs: _asInt(json['expires_at_ms']) ?? 0,
      lastSeenAtMs: _asInt(json['last_seen_at_ms']) ?? 0,
    );
  }
}

class BackendConfig {
  final EngineConfig engine;
  final ProcessConfig process;
  final PaperlessConfig paperless;
  final LLMConfig llms;

  BackendConfig({
    required this.engine,
    required this.process,
    required this.paperless,
    required this.llms,
  });

  factory BackendConfig.fromJson(Map<String, dynamic> json) {
    return BackendConfig(
      engine: EngineConfig.fromJson(json['engine'] ?? {}),
      process: ProcessConfig.fromJson(json['process'] ?? {}),
      paperless: PaperlessConfig.fromJson(json['paperless'] ?? {}),
      llms: LLMConfig.fromJson(json['llms'] ?? {}),
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'engine': engine.toJson(),
      'process': process.toJson(),
      'paperless': paperless.toJson(),
      'llms': llms.toJson(),
    };
  }
}

class EngineConfig {
  final String processingMode;
  final int processingIntervalSeconds;

  EngineConfig({
    required this.processingMode,
    required this.processingIntervalSeconds,
  });

  factory EngineConfig.fromJson(Map<String, dynamic> json) {
    return EngineConfig(
      processingMode: _asString(json['processing_mode']) ?? 'manual',
      processingIntervalSeconds: _asInt(json['processing_interval']) ?? 30,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'processing_mode': processingMode,
      'processing_interval': processingIntervalSeconds,
    };
  }
}

class ProcessConfig {
  final String processTriggerTag;
  final String forceOcrTag;
  final String forceVisionTag;
  final String processCorrespondentTag;
  final String processDocumentTypeTag;
  final String processDocumentTagsTag;
  final String processCompletedTag;

  ProcessConfig({
    required this.processTriggerTag,
    required this.forceOcrTag,
    required this.forceVisionTag,
    required this.processCorrespondentTag,
    required this.processDocumentTypeTag,
    required this.processDocumentTagsTag,
    required this.processCompletedTag,
  });

  factory ProcessConfig.fromJson(Map<String, dynamic> json) {
    return ProcessConfig(
      processTriggerTag: _asString(json['process_trigger_tag']) ?? '',
      forceOcrTag: _asString(json['force_ocr_tag']) ?? '',
      forceVisionTag: _asString(json['force_vision_tag']) ?? '',
      processCorrespondentTag:
          _asString(json['process_correspondent_tag']) ?? '',
      processDocumentTypeTag:
          _asString(json['process_document_type_tag']) ?? '',
      processDocumentTagsTag:
          _asString(json['process_document_tags_tag']) ?? '',
      processCompletedTag: _asString(json['process_completed_tag']) ?? '',
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'process_trigger_tag': processTriggerTag,
      'force_ocr_tag': forceOcrTag,
      'force_vision_tag': forceVisionTag,
      'process_correspondent_tag': processCorrespondentTag,
      'process_document_type_tag': processDocumentTypeTag,
      'process_document_tags_tag': processDocumentTagsTag,
      'process_completed_tag': processCompletedTag,
    };
  }
}

class PaperlessConfig {
  final String paperlessUrl;
  final String paperlessToken;

  PaperlessConfig({required this.paperlessUrl, required this.paperlessToken});

  factory PaperlessConfig.fromJson(Map<String, dynamic> json) {
    return PaperlessConfig(
      paperlessUrl: _asString(json['paperless_url']) ?? '',
      paperlessToken: _asString(json['paperless_token']) ?? '',
    );
  }

  Map<String, dynamic> toJson() {
    return {'paperless_url': paperlessUrl, 'paperless_token': paperlessToken};
  }
}

class LLMConfig {
  final String ollamaUrl;
  final String defaultLlm;
  final String visionLlm;

  LLMConfig({
    required this.ollamaUrl,
    required this.defaultLlm,
    required this.visionLlm,
  });

  factory LLMConfig.fromJson(Map<String, dynamic> json) {
    return LLMConfig(
      ollamaUrl: _asString(json['ollama_url']) ?? '',
      defaultLlm: _asString(json['default_llm']) ?? '',
      visionLlm: _asString(json['vision_llm']) ?? '',
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'ollama_url': ollamaUrl,
      'default_llm': defaultLlm,
      'vision_llm': visionLlm,
    };
  }
}

class QueueItem {
  final int id;
  final int? documentId;
  final String documentTitle;
  final String source;
  final String status;
  final String? payload;
  final int requestedAtMs;
  final int? startedAtMs;
  final int? completedAtMs;
  final int attempts;
  final String? lastError;
  final String? resultSummary;
  final String? usedLlm;
  final String? usedVisionLlm;
  final int? processingDurationMs;

  QueueItem({
    required this.id,
    this.documentId,
    required this.documentTitle,
    required this.source,
    required this.status,
    this.payload,
    required this.requestedAtMs,
    this.startedAtMs,
    this.completedAtMs,
    required this.attempts,
    this.lastError,
    this.resultSummary,
    this.usedLlm,
    this.usedVisionLlm,
    this.processingDurationMs,
  });

  factory QueueItem.fromJson(Map<String, dynamic> json) {
    return QueueItem(
      id: _asInt(json['id']) ?? 0,
      documentId: _asInt(json['document_id']),
      documentTitle: _asString(json['document_title']) ?? 'Untitled',
      source: _asString(json['source']) ?? '',
      status: _asString(json['status']) ?? 'pending',
      payload: _asString(json['payload']),
      requestedAtMs: _asInt(json['requested_at_ms']) ?? 0,
      startedAtMs: _asInt(json['started_at_ms']),
      completedAtMs: _asInt(json['completed_at_ms']),
      attempts: _asInt(json['attempts']) ?? 0,
      lastError: _asString(json['last_error']),
      resultSummary: _asString(json['result_summary']),
      usedLlm: _asString(json['used_llm']),
      usedVisionLlm: _asString(json['used_vision_llm']),
      processingDurationMs: _asInt(json['processing_duration_ms']),
    );
  }
}

class DashboardStats {
  final int queuedCount;
  final double averageProcessingMs;
  final double processingSuccessRate;
  final List<QueueItem> recentRuns;

  DashboardStats({
    required this.queuedCount,
    required this.averageProcessingMs,
    required this.processingSuccessRate,
    required this.recentRuns,
  });

  factory DashboardStats.fromJson(Map<String, dynamic> json) {
    var rawRuns = json['recent_runs'];
    List<QueueItem> runs = [];
    if (rawRuns is List) {
      runs = rawRuns.map((e) => QueueItem.fromJson(e)).toList();
    }
    return DashboardStats(
      queuedCount: _asInt(json['queued_count']) ?? 0,
      averageProcessingMs: _asDouble(json['average_processing_time_ms']) ?? 0.0,
      processingSuccessRate: _asDouble(json['processing_success_rate']) ?? 0.0,
      recentRuns: runs,
    );
  }
}

class OllamaModelDetails {
  final String parentModel;
  final String format;
  final String family;
  final List<String> families;
  final String parameterSize;
  final String quantizationLevel;

  OllamaModelDetails({
    required this.parentModel,
    required this.format,
    required this.family,
    required this.families,
    required this.parameterSize,
    required this.quantizationLevel,
  });

  factory OllamaModelDetails.fromJson(Map<String, dynamic> json) {
    var rawFamilies = json['families'];
    List<String> fams = [];
    if (rawFamilies is List) {
      fams = rawFamilies.map((e) => _asString(e) ?? '').toList();
    }
    return OllamaModelDetails(
      parentModel: _asString(json['parent_model']) ?? '',
      format: _asString(json['format']) ?? '',
      family: _asString(json['family']) ?? '',
      families: fams,
      parameterSize: _asString(json['parameter_size']) ?? '',
      quantizationLevel: _asString(json['quantization_level']) ?? '',
    );
  }
}

class OllamaModel {
  final String name;
  final String model;
  final String modifiedAt;
  final int size;
  final String digest;
  final OllamaModelDetails? details;

  OllamaModel({
    required this.name,
    required this.model,
    required this.modifiedAt,
    required this.size,
    required this.digest,
    this.details,
  });

  factory OllamaModel.fromJson(Map<String, dynamic> json) {
    return OllamaModel(
      name: _asString(json['name']) ?? '',
      model: _asString(json['model']) ?? '',
      modifiedAt: _asString(json['modified_at']) ?? '',
      size: _asInt(json['size']) ?? 0,
      digest: _asString(json['digest']) ?? '',
      details: json['details'] != null
          ? OllamaModelDetails.fromJson(json['details'])
          : null,
    );
  }
}

class DocumentTag {
  final int id;
  final String name;

  DocumentTag({
    required this.id,
    required this.name,
  });

  factory DocumentTag.fromJson(Map<String, dynamic> json) {
    return DocumentTag(
      id: _asInt(json['id']) ?? 0,
      name: _asString(json['name']) ?? '',
    );
  }
}

class OllamaModelsResponse {
  final List<OllamaModel> models;

  OllamaModelsResponse({required this.models});

  factory OllamaModelsResponse.fromJson(Map<String, dynamic> json) {
    var rawModels = json['models'];
    List<OllamaModel> mods = [];
    if (rawModels is List) {
      mods = rawModels.map((e) => OllamaModel.fromJson(e)).toList();
    }
    return OllamaModelsResponse(models: mods);
  }
}

String? _asString(dynamic val) {
  if (val == null) return null;
  return val.toString();
}

int? _asInt(dynamic val) {
  if (val == null) return null;
  if (val is int) return val;
  if (val is double) return val.toInt();
  if (val is String) return int.tryParse(val);
  return null;
}

double? _asDouble(dynamic val) {
  if (val == null) return null;
  if (val is double) return val;
  if (val is int) return val.toDouble();
  if (val is String) return double.tryParse(val);
  return null;
}
