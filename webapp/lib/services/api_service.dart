import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';
import 'package:webapp/models/models.dart';

class ApiException implements Exception {
  final String message;
  final int statusCode;
  ApiException(this.message, this.statusCode);
  @override
  String toString() => 'ApiException: \$message (Status: \$statusCode)';
}

class ApiService {
  static const String baseUrl = 'http://localhost:8080/api';

  SharedPreferences? _prefs;

  Future<void> init() async {
    _prefs = await SharedPreferences.getInstance();
  }

  String? get token => _prefs?.getString('auth_token');

  Map<String, String> get _authHeaders {
    final t = token;
    return {
      'Content-Type': 'application/json',
      if (t != null && t.isNotEmpty) 'Authorization': 'Bearer $t',
    };
  }

  Future<void> _handleErrors(http.Response response) async {
    if (response.statusCode >= 400) {
      String msg = 'Request failed';
      try {
        final body = jsonDecode(response.body);
        msg = body['error'] ?? msg;
      } catch (_) {}
      throw ApiException(msg, response.statusCode);
    }
  }

  Future<Session> login(String username, String password) async {
    final response = await http.post(
      Uri.parse(baseUrl + '/auth/login'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({'username': username, 'password': password}),
    );

    await _handleErrors(response);
    final data = jsonDecode(response.body);
    final session = Session(
      token: data['token'] ?? '',
      username: data['username'] ?? '',
      expiresAtMs: data['expires_at_ms'] ?? 0,
      createdAtMs: DateTime.now().millisecondsSinceEpoch,
      lastSeenAtMs: DateTime.now().millisecondsSinceEpoch,
    );

    await _prefs?.setString('auth_token', session.token);
    return session;
  }

  Future<void> logout() async {
    try {
      await http.post(
        Uri.parse(baseUrl + '/auth/logout'),
        headers: _authHeaders,
      );
    } catch (_) {}
    await _prefs?.remove('auth_token');
  }

  Future<Session> getMe() async {
    final response = await http.get(
      Uri.parse(baseUrl + '/auth/me'),
      headers: _authHeaders,
    );
    await _handleErrors(response);
    return Session.fromJson(jsonDecode(response.body));
  }

  Future<DashboardStats> getDashboard() async {
    final response = await http.get(
      Uri.parse(baseUrl + '/dashboard'),
      headers: _authHeaders,
    );
    await _handleErrors(response);
    return DashboardStats.fromJson(jsonDecode(response.body));
  }

  Future<BackendConfig> getConfig() async {
    final response = await http.get(
      Uri.parse(baseUrl + '/config'),
      headers: _authHeaders,
    );
    await _handleErrors(response);
    return BackendConfig.fromJson(jsonDecode(response.body));
  }

  Future<BackendConfig> putConfig(BackendConfig config) async {
    final response = await http.put(
      Uri.parse(baseUrl + '/config'),
      headers: _authHeaders,
      body: jsonEncode(config.toJson()),
    );
    await _handleErrors(response);
    return BackendConfig.fromJson(jsonDecode(response.body));
  }

  Future<List<QueueItem>> getQueue({String? status, int limit = 50}) async {
    var qs = '?limit=${limit.toString()}';
    if (status != null && status.isNotEmpty) qs += '&status=$status';
    final response = await http.get(
      Uri.parse('${baseUrl}/queue${qs}'),
      headers: _authHeaders,
    );
    await _handleErrors(response);
    final data = jsonDecode(response.body);
    final items = data['items'] as List;
    return items.map((e) => QueueItem.fromJson(e)).toList();
  }

  Future<QueueItem> processQueueItem(int id) async {
    final response = await http.post(
      Uri.parse(baseUrl + '/queue/' + id.toString() + '/process'),
      headers: _authHeaders,
    );
    await _handleErrors(response);
    return QueueItem.fromJson(jsonDecode(response.body));
  }
  Future<OllamaModelsResponse> getModels() async {
    final response = await http.get(
      Uri.parse(baseUrl + '/models'),
      headers: _authHeaders,
    );
    await _handleErrors(response);
    return OllamaModelsResponse.fromJson(jsonDecode(response.body));
  }
}
