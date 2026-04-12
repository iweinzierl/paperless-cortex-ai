class Session {
  final String token;
  final String username;
  final int createdAtMs;
  final int expiresAtMs;
  final int lastSeenAtMs;

  const Session({
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

class QueueItem {
  final int id;
  final int? documentId;
  final String documentTitle;
  final String source;
  final String status;
  final int requestedAtMs;
  final String? lastError;
  final String? resultSummary;
  final String applyStatus;

  const QueueItem({
    required this.id,
    required this.documentId,
    required this.documentTitle,
    required this.source,
    required this.status,
    required this.requestedAtMs,
    required this.lastError,
    required this.resultSummary,
    required this.applyStatus,
  });

  factory QueueItem.fromJson(Map<String, dynamic> json) {
    return QueueItem(
      id: _asInt(json['id']) ?? 0,
      documentId: _asInt(json['document_id']),
      documentTitle: _asString(json['document_title']) ?? 'Untitled',
      source: _asString(json['source']) ?? '',
      status: _asString(json['status']) ?? 'pending',
      requestedAtMs: _asInt(json['requested_at_ms']) ?? 0,
      lastError: _asString(json['last_error']),
      resultSummary: _asString(json['result_summary']),
      applyStatus: _asString(json['apply_status']) ?? '',
    );
  }
}

class DashboardStats {
  final int queuedCount;
  final double averageProcessingMs;
  final double processingSuccessRate;
  final List<QueueItem> recentRuns;

  const DashboardStats({
    required this.queuedCount,
    required this.averageProcessingMs,
    required this.processingSuccessRate,
    required this.recentRuns,
  });

  factory DashboardStats.fromJson(Map<String, dynamic> json) {
    final rawRuns = json['recent_runs'];
    var recentRuns = const <QueueItem>[];
    if (rawRuns is List) {
      recentRuns = rawRuns
          .whereType<Map>()
          .map((item) => QueueItem.fromJson(item.cast<String, dynamic>()))
          .toList(growable: false);
    }
    return DashboardStats(
      queuedCount: _asInt(json['queued_count']) ?? 0,
      averageProcessingMs: _asDouble(json['average_processing_time_ms']) ?? 0,
      processingSuccessRate: _asDouble(json['processing_success_rate']) ?? 0,
      recentRuns: recentRuns,
    );
  }
}

String? _asString(dynamic value) {
  if (value == null) {
    return null;
  }
  if (value is String) {
    return value;
  }
  return value.toString();
}

int? _asInt(dynamic value) {
  if (value == null) {
    return null;
  }
  if (value is int) {
    return value;
  }
  if (value is num) {
    return value.toInt();
  }
  return int.tryParse(value.toString());
}

double? _asDouble(dynamic value) {
  if (value == null) {
    return null;
  }
  if (value is double) {
    return value;
  }
  if (value is num) {
    return value.toDouble();
  }
  return double.tryParse(value.toString());
}

Map<String, dynamic>? _asMap(dynamic value) {
  if (value is Map<String, dynamic>) {
    return value;
  }
  if (value is Map) {
    return value.cast<String, dynamic>();
  }
  return null;
}
