import 'dart:convert';

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

class DependencyStatusModel {
  final bool configured;
  final bool healthy;
  final String message;
  final int modelCount;

  const DependencyStatusModel({
    required this.configured,
    required this.healthy,
    required this.message,
    this.modelCount = 0,
  });

  factory DependencyStatusModel.fromJson(Map<String, dynamic> json) {
    return DependencyStatusModel(
      configured: json['configured'] == true,
      healthy: json['healthy'] == true,
      message: _asString(json['message']) ?? '',
      modelCount: _asInt(json['model_count']) ?? 0,
    );
  }
}

class SystemStatusModel {
  final DependencyStatusModel backend;
  final DependencyStatusModel paperless;
  final DependencyStatusModel ollama;

  const SystemStatusModel({
    required this.backend,
    required this.paperless,
    required this.ollama,
  });

  factory SystemStatusModel.fromJson(Map<String, dynamic> json) {
    return SystemStatusModel(
      backend: DependencyStatusModel.fromJson(_asMap(json['backend']) ?? {}),
      paperless: DependencyStatusModel.fromJson(
        _asMap(json['paperless']) ?? {},
      ),
      ollama: DependencyStatusModel.fromJson(_asMap(json['ollama']) ?? {}),
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
  final String processCreatedDateTag;
  final String processCorrespondentTag;
  final String processDocumentTypeTag;
  final String processDocumentTagsTag;
  final String processTitleTag;
  final String processCompletedTag;

  ProcessConfig({
    required this.processTriggerTag,
    required this.forceOcrTag,
    required this.forceVisionTag,
    required this.processCreatedDateTag,
    required this.processCorrespondentTag,
    required this.processDocumentTypeTag,
    required this.processDocumentTagsTag,
    required this.processTitleTag,
    required this.processCompletedTag,
  });

  factory ProcessConfig.fromJson(Map<String, dynamic> json) {
    return ProcessConfig(
      processTriggerTag: _asString(json['process_trigger_tag']) ?? '',
      forceOcrTag: _asString(json['force_ocr_tag']) ?? '',
      forceVisionTag: _asString(json['force_vision_tag']) ?? '',
      processCreatedDateTag: _asString(json['process_created_date_tag']) ?? '',
      processCorrespondentTag:
          _asString(json['process_correspondent_tag']) ?? '',
      processDocumentTypeTag:
          _asString(json['process_document_type_tag']) ?? '',
      processDocumentTagsTag:
          _asString(json['process_document_tags_tag']) ?? '',
      processTitleTag: _asString(json['process_title_tag']) ?? '',
      processCompletedTag: _asString(json['process_completed_tag']) ?? '',
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'process_trigger_tag': processTriggerTag,
      'force_ocr_tag': forceOcrTag,
      'force_vision_tag': forceVisionTag,
      'process_created_date_tag': processCreatedDateTag,
      'process_correspondent_tag': processCorrespondentTag,
      'process_document_type_tag': processDocumentTypeTag,
      'process_document_tags_tag': processDocumentTagsTag,
      'process_title_tag': processTitleTag,
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

class ProcessingStageProgress {
  final String key;
  final String label;
  final String status;
  final String? detail;
  final String? usedModel;

  const ProcessingStageProgress({
    required this.key,
    required this.label,
    required this.status,
    this.detail,
    this.usedModel,
  });

  bool get isRequested => status != 'skipped';
  bool get isPending => status == 'pending';
  bool get isRunning => status == 'running';
  bool get isCompleted => status == 'completed';
  bool get isFailed => status == 'failed';
}

class ProcessingPlanModel {
  final bool triggerTagPresent;
  final bool forceOcr;
  final bool forceVision;
  final bool processCreatedDate;
  final bool processCorrespondent;
  final bool processDocumentType;
  final bool processDocumentTags;
  final bool processTitle;
  final List<String> requestedStages;

  ProcessingPlanModel({
    required this.triggerTagPresent,
    required this.forceOcr,
    required this.forceVision,
    required this.processCreatedDate,
    required this.processCorrespondent,
    required this.processDocumentType,
    required this.processDocumentTags,
    required this.processTitle,
    required this.requestedStages,
  });

  factory ProcessingPlanModel.fromJson(Map<String, dynamic> json) {
    final rawRequestedStages = json['requested_stages'];
    List<String> requestedStages = [];
    if (rawRequestedStages is List) {
      requestedStages = rawRequestedStages
          .map((entry) => _asString(entry) ?? '')
          .where((entry) => entry.isNotEmpty)
          .toList();
    }

    return ProcessingPlanModel(
      triggerTagPresent: json['trigger_tag_present'] == true,
      forceOcr: json['force_ocr'] == true,
      forceVision: json['force_vision'] == true,
      processCreatedDate: json['process_created_date'] == true,
      processCorrespondent: json['process_correspondent'] == true,
      processDocumentType: json['process_document_type'] == true,
      processDocumentTags: json['process_document_tags'] == true,
      processTitle: json['process_title'] == true,
      requestedStages: requestedStages,
    );
  }
}

class ExtractionStageModel {
  final String status;
  final String error;
  final String source;
  final String usedModel;
  final int textLength;
  final String textPreview;

  ExtractionStageModel({
    required this.status,
    required this.error,
    required this.source,
    required this.usedModel,
    required this.textLength,
    required this.textPreview,
  });

  factory ExtractionStageModel.fromJson(Map<String, dynamic> json) {
    return ExtractionStageModel(
      status: _asString(json['status']) ?? 'skipped',
      error: _asString(json['error']) ?? '',
      source: _asString(json['source']) ?? '',
      usedModel: _asString(json['used_model']) ?? '',
      textLength: _asInt(json['text_length']) ?? 0,
      textPreview: _asString(json['text_preview']) ?? '',
    );
  }

  ProcessingStageProgress toStageProgress() {
    String? detail;
    if (error.isNotEmpty) {
      detail = error;
    } else if (source.isNotEmpty) {
      detail = 'Source: $source';
    } else if (textLength > 0) {
      detail = '$textLength characters extracted';
    }

    return ProcessingStageProgress(
      key: 'extract_text',
      label: 'Text Extraction',
      status: status,
      detail: detail,
      usedModel: usedModel.isEmpty ? null : usedModel,
    );
  }
}

class SuggestionStageModel {
  final String status;
  final String error;
  final String usedModel;
  final String confidence;
  final String reasoning;
  final Map<String, dynamic>? payload;

  SuggestionStageModel({
    required this.status,
    required this.error,
    required this.usedModel,
    required this.confidence,
    required this.reasoning,
    this.payload,
  });

  factory SuggestionStageModel.fromJson(Map<String, dynamic> json) {
    return SuggestionStageModel(
      status: _asString(json['status']) ?? 'skipped',
      error: _asString(json['error']) ?? '',
      usedModel: _asString(json['used_model']) ?? '',
      confidence: _asString(json['confidence']) ?? '',
      reasoning: _asString(json['reasoning']) ?? '',
      payload: _asMap(json['payload']),
    );
  }

  ProcessingStageProgress toStageProgress(String key, String label) {
    String? detail;
    if (error.isNotEmpty) {
      detail = error;
    } else if (confidence.isNotEmpty) {
      detail = 'Confidence: $confidence';
    } else if (reasoning.isNotEmpty) {
      detail = reasoning;
    }

    return ProcessingStageProgress(
      key: key,
      label: label,
      status: status,
      detail: detail,
      usedModel: usedModel.isEmpty ? null : usedModel,
    );
  }
}

class ProcessingResultModel {
  final ProcessingDocumentModel? document;
  final ProcessingPlanModel? plan;
  final ExtractionStageModel extraction;
  final SuggestionStageModel createdDate;
  final SuggestionStageModel correspondent;
  final SuggestionStageModel documentType;
  final SuggestionStageModel tags;
  final SuggestionStageModel title;
  final List<String> notes;

  ProcessingResultModel({
    required this.document,
    required this.plan,
    required this.extraction,
    required this.createdDate,
    required this.correspondent,
    required this.documentType,
    required this.tags,
    required this.title,
    required this.notes,
  });

  factory ProcessingResultModel.fromJson(Map<String, dynamic> json) {
    final rawNotes = json['notes'];
    List<String> notes = [];
    if (rawNotes is List) {
      notes = rawNotes
          .map((entry) => _asString(entry) ?? '')
          .where((entry) => entry.isNotEmpty)
          .toList();
    }

    return ProcessingResultModel(
      document: _asMap(json['document']) != null
          ? ProcessingDocumentModel.fromJson(_asMap(json['document'])!)
          : null,
      plan: _asMap(json['plan']) != null
          ? ProcessingPlanModel.fromJson(_asMap(json['plan'])!)
          : null,
      extraction: ExtractionStageModel.fromJson(
        _asMap(json['extraction']) ?? {},
      ),
      createdDate: SuggestionStageModel.fromJson(
        _asMap(json['created_date']) ?? {},
      ),
      correspondent: SuggestionStageModel.fromJson(
        _asMap(json['correspondent']) ?? {},
      ),
      documentType: SuggestionStageModel.fromJson(
        _asMap(json['document_type']) ?? {},
      ),
      tags: SuggestionStageModel.fromJson(_asMap(json['tags']) ?? {}),
      title: SuggestionStageModel.fromJson(_asMap(json['title']) ?? {}),
      notes: notes,
    );
  }

  List<ProcessingStageProgress> get stages => [
    extraction.toStageProgress(),
    createdDate.toStageProgress('created_date', 'Created Date'),
    correspondent.toStageProgress('correspondent', 'Correspondent'),
    documentType.toStageProgress('document_type', 'Document Type'),
    tags.toStageProgress('tags', 'Tags'),
    title.toStageProgress('title', 'Title'),
  ];

  List<ProcessingStageProgress> get requestedStages =>
      stages.where((stage) => stage.isRequested).toList(growable: false);

  int get requestedStageCount => requestedStages.length;

  int get completedStageCount =>
      requestedStages.where((stage) => stage.isCompleted).length;

  bool get hasFailure => requestedStages.any((stage) => stage.isFailed);

  bool get hasCompletedSuggestion =>
      createdDate.status == 'completed' ||
      correspondent.status == 'completed' ||
      documentType.status == 'completed' ||
      tags.status == 'completed' ||
      title.status == 'completed';

  ProcessingStageProgress? get activeStage {
    for (final stage in requestedStages) {
      if (stage.isRunning) {
        return stage;
      }
    }
    for (final stage in requestedStages) {
      if (stage.isPending) {
        return stage;
      }
    }
    for (final stage in requestedStages) {
      if (stage.isFailed) {
        return stage;
      }
    }
    return null;
  }

  double get completionFraction {
    final total = requestedStageCount;
    if (total == 0) {
      return 0;
    }

    final hasRunningStage = requestedStages.any((stage) => stage.isRunning);
    final fraction =
        (completedStageCount + (hasRunningStage ? 0.5 : 0.0)) / total;
    return fraction.clamp(0.0, 1.0);
  }

  String? get failureSummary {
    for (final stage in requestedStages) {
      if (stage.isFailed && stage.detail != null && stage.detail!.isNotEmpty) {
        return stage.detail;
      }
    }
    return null;
  }

  String? get extractionPreview {
    if (extraction.textPreview.isEmpty) {
      return null;
    }
    return extraction.textPreview;
  }
}

class ProcessingDocumentModel {
  final int id;
  final String title;
  final String created;
  final String added;
  final int? correspondentId;
  final String correspondentName;
  final int? documentTypeId;
  final String documentTypeName;

  ProcessingDocumentModel({
    required this.id,
    required this.title,
    required this.created,
    required this.added,
    this.correspondentId,
    required this.correspondentName,
    this.documentTypeId,
    required this.documentTypeName,
  });

  factory ProcessingDocumentModel.fromJson(Map<String, dynamic> json) {
    return ProcessingDocumentModel(
      id: _asInt(json['id']) ?? 0,
      title: _asString(json['title']) ?? '',
      created: _asString(json['created']) ?? '',
      added: _asString(json['added']) ?? '',
      correspondentId: _asInt(json['correspondent_id']),
      correspondentName: _asString(json['correspondent_name']) ?? '',
      documentTypeId: _asInt(json['document_type_id']),
      documentTypeName: _asString(json['document_type_name']) ?? '',
    );
  }
}

class QueueItem {
  final int id;
  final int? documentId;
  final String documentTitle;
  final String source;
  final String status;
  final String? payload;
  final List<String> requestedStageKeys;
  final int requestedAtMs;
  final int? startedAtMs;
  final int? completedAtMs;
  final int attempts;
  final String? lastError;
  final String? resultSummary;
  final String? resultPayload;
  final ProcessingResultModel? processingResult;
  final String? usedLlm;
  final String? usedVisionLlm;
  final int? processingDurationMs;
  final String applyStatus;
  final int? appliedAtMs;
  final String? applyError;
  final String? appliedSummary;

  QueueItem({
    required this.id,
    this.documentId,
    required this.documentTitle,
    required this.source,
    required this.status,
    this.payload,
    required this.requestedStageKeys,
    required this.requestedAtMs,
    this.startedAtMs,
    this.completedAtMs,
    required this.attempts,
    this.lastError,
    this.resultSummary,
    this.resultPayload,
    this.processingResult,
    this.usedLlm,
    this.usedVisionLlm,
    this.processingDurationMs,
    required this.applyStatus,
    this.appliedAtMs,
    this.applyError,
    this.appliedSummary,
  });

  factory QueueItem.fromJson(Map<String, dynamic> json) {
    return QueueItem(
      id: _asInt(json['id']) ?? 0,
      documentId: _asInt(json['document_id']),
      documentTitle: _asString(json['document_title']) ?? 'Untitled',
      source: _asString(json['source']) ?? '',
      status: _asString(json['status']) ?? 'pending',
      payload: _asString(json['payload']),
      requestedStageKeys: _asStringList(json['requested_stages']),
      requestedAtMs: _asInt(json['requested_at_ms']) ?? 0,
      startedAtMs: _asInt(json['started_at_ms']),
      completedAtMs: _asInt(json['completed_at_ms']),
      attempts: _asInt(json['attempts']) ?? 0,
      lastError: _asString(json['last_error']),
      resultSummary: _asString(json['result_summary']),
      resultPayload: _asString(json['result_payload']),
      processingResult: _parseProcessingResult(json['result_payload']),
      usedLlm: _asString(json['used_llm']),
      usedVisionLlm: _asString(json['used_vision_llm']),
      processingDurationMs: _asInt(json['processing_duration_ms']),
      applyStatus: _asString(json['apply_status']) ?? '',
      appliedAtMs: _asInt(json['applied_at_ms']),
      applyError: _asString(json['apply_error']),
      appliedSummary: _asString(json['applied_summary']),
    );
  }

  bool get isProcessing => status == 'processing';

  bool get isPartiallyCompleted => status == 'partially_completed';

  bool get isApplied => applyStatus == 'applied';

  bool get hasApplyFailure => applyStatus == 'failed';

  bool get canViewResultDetails =>
      status == 'completed' ||
      status == 'failed' ||
      status == 'partially_completed';

  bool get canRemoveFromQueue => status != 'processing';

  bool canApplySuggestions({
    required bool isManualMode,
    required bool hasCompletedTag,
  }) {
    if (!isManualMode || isApplied) {
      return false;
    }
    if (status != 'completed' && status != 'partially_completed') {
      return false;
    }
    return (processingResult?.hasCompletedSuggestion ?? false) ||
        hasCompletedTag;
  }

  List<ProcessingStageProgress> get requestedStages {
    if (requestedStageKeys.isNotEmpty) {
      return requestedStageKeys
          .map(_queueStageProgressFromKey)
          .whereType<ProcessingStageProgress>()
          .toList(growable: false);
    }
    return processingResult?.requestedStages ??
        const <ProcessingStageProgress>[];
  }

  DateTime get requestedAt =>
      DateTime.fromMillisecondsSinceEpoch(requestedAtMs);

  DateTime? get completedAt => completedAtMs == null
      ? null
      : DateTime.fromMillisecondsSinceEpoch(completedAtMs!);

  DateTime? get appliedAt => appliedAtMs == null
      ? null
      : DateTime.fromMillisecondsSinceEpoch(appliedAtMs!);

  String get detailSummary {
    final error = lastError?.trim();
    if (error != null && error.isNotEmpty) {
      return error;
    }
    final summary = resultSummary?.trim();
    if (summary != null && summary.isNotEmpty) {
      return summary;
    }
    final failure = processingResult?.failureSummary?.trim();
    if (failure != null && failure.isNotEmpty) {
      return failure;
    }
    final notes = processingResult?.notes;
    if (notes != null && notes.isNotEmpty) {
      return notes.first;
    }
    return 'No detailed process result available.';
  }

  Duration? get elapsedDuration {
    final start = startedAtMs;
    if (start == null) {
      return null;
    }

    final end = completedAtMs ?? DateTime.now().millisecondsSinceEpoch;
    final milliseconds = end >= start ? end - start : 0;
    return Duration(milliseconds: milliseconds);
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

  DocumentTag({required this.id, required this.name});

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

ProcessingResultModel? _parseProcessingResult(dynamic value) {
  try {
    if (value == null) {
      return null;
    }
    if (value is String) {
      if (value.trim().isEmpty) {
        return null;
      }
      final decoded = jsonDecode(value);
      final map = _asMap(decoded);
      if (map == null) {
        return null;
      }
      return ProcessingResultModel.fromJson(map);
    }
    final map = _asMap(value);
    if (map == null) {
      return null;
    }
    return ProcessingResultModel.fromJson(map);
  } catch (_) {
    return null;
  }
}

Map<String, dynamic>? _asMap(dynamic val) {
  if (val is Map<String, dynamic>) {
    return val;
  }
  if (val is Map) {
    return val.map((key, value) => MapEntry(key.toString(), value));
  }
  return null;
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

List<String> _asStringList(dynamic val) {
  if (val is! List) {
    return const <String>[];
  }
  return val
      .map((entry) => _asString(entry)?.trim() ?? '')
      .where((entry) => entry.isNotEmpty)
      .toList(growable: false);
}

ProcessingStageProgress? _queueStageProgressFromKey(String key) {
  switch (key) {
    case 'extract_text':
      return const ProcessingStageProgress(
        key: 'extract_text',
        label: 'OCR / Text',
        status: 'pending',
      );
    case 'created_date':
      return const ProcessingStageProgress(
        key: 'created_date',
        label: 'Created Date',
        status: 'pending',
      );
    case 'correspondent':
      return const ProcessingStageProgress(
        key: 'correspondent',
        label: 'Correspondent',
        status: 'pending',
      );
    case 'document_type':
      return const ProcessingStageProgress(
        key: 'document_type',
        label: 'Document Type',
        status: 'pending',
      );
    case 'tags':
      return const ProcessingStageProgress(
        key: 'tags',
        label: 'Tags',
        status: 'pending',
      );
    case 'title':
      return const ProcessingStageProgress(
        key: 'title',
        label: 'Title',
        status: 'pending',
      );
    default:
      return null;
  }
}
