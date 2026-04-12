import 'dart:convert';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';

import 'package:app/models/app_models.dart';

class ApiException implements Exception {
  final String message;
  final int statusCode;

  const ApiException(this.message, this.statusCode);

  @override
  String toString() => 'ApiException: $message (status: $statusCode)';
}

class ApiService {
  static const String _configuredBaseUrl = String.fromEnvironment(
    'API_BASE_URL',
    defaultValue: '',
  );

  SharedPreferences? _prefs;

  static String get baseUrl {
    final configured = _configuredBaseUrl.trim();
    if (configured.isNotEmpty) {
      return configured;
    }

    if (kIsWeb) {
      final current = Uri.base;
      final isLocalHost =
          current.host == 'localhost' ||
          current.host == '127.0.0.1' ||
          current.host == '::1';
      if (isLocalHost && current.port != 8080) {
        return 'http://${current.host}:8080/api';
      }
      return '/api';
    }

    if (Platform.isAndroid) {
      return 'http://10.0.2.2:8080/api';
    }

    return 'http://localhost:8080/api';
  }

  Future<void> init() async {
    _prefs = await SharedPreferences.getInstance();
  }

  String? get token => _prefs?.getString('auth_token');

  Map<String, String> get _authHeaders {
    final authToken = token;
    return {
      'Content-Type': 'application/json',
      if (authToken != null && authToken.isNotEmpty)
        'Authorization': 'Bearer $authToken',
    };
  }

  Future<void> _handleErrors(http.Response response) async {
    if (response.statusCode < 400) {
      return;
    }

    var message = 'Request failed';
    try {
      final body = jsonDecode(response.body);
      if (body is Map<String, dynamic>) {
        message = body['error']?.toString() ?? message;
      }
    } catch (_) {}
    throw ApiException(message, response.statusCode);
  }

  Future<Session> login(String username, String password) async {
    final response = await http.post(
      Uri.parse('$baseUrl/auth/login'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({'username': username, 'password': password}),
    );

    await _handleErrors(response);
    final payload = jsonDecode(response.body) as Map<String, dynamic>;
    final session = Session(
      token: payload['token']?.toString() ?? '',
      username: payload['username']?.toString() ?? username,
      createdAtMs: DateTime.now().millisecondsSinceEpoch,
      expiresAtMs: payload['expires_at_ms'] as int? ?? 0,
      lastSeenAtMs: DateTime.now().millisecondsSinceEpoch,
    );

    await _prefs?.setString('auth_token', session.token);
    return session;
  }

  Future<void> logout() async {
    try {
      await http.post(Uri.parse('$baseUrl/auth/logout'), headers: _authHeaders);
    } catch (_) {}
    await _prefs?.remove('auth_token');
  }

  Future<Session> getMe() async {
    final response = await http.get(
      Uri.parse('$baseUrl/auth/me'),
      headers: _authHeaders,
    );
    await _handleErrors(response);
    return Session.fromJson(jsonDecode(response.body) as Map<String, dynamic>);
  }

  Future<SystemStatusModel> getSystemStatus() async {
    final response = await http.get(
      Uri.parse('$baseUrl/status'),
      headers: _authHeaders,
    );
    await _handleErrors(response);
    return SystemStatusModel.fromJson(
      jsonDecode(response.body) as Map<String, dynamic>,
    );
  }

  Future<DashboardStats> getDashboard() async {
    final response = await http.get(
      Uri.parse('$baseUrl/dashboard'),
      headers: _authHeaders,
    );
    await _handleErrors(response);
    return DashboardStats.fromJson(
      jsonDecode(response.body) as Map<String, dynamic>,
    );
  }

  Future<List<QueueItem>> getQueue({String? status, int limit = 30}) async {
    var query = '?limit=$limit';
    if (status != null && status.isNotEmpty) {
      query += '&status=$status';
    }

    final response = await http.get(
      Uri.parse('$baseUrl/queue$query'),
      headers: _authHeaders,
    );
    await _handleErrors(response);

    final payload = jsonDecode(response.body) as Map<String, dynamic>;
    final items = payload['items'];
    if (items is! List) {
      return const <QueueItem>[];
    }
    return items
        .whereType<Map>()
        .map((item) => QueueItem.fromJson(item.cast<String, dynamic>()))
        .toList(growable: false);
  }
}
